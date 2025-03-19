import * as vpcmod from '@pulumi/vpcmod';
import * as k8s from '@pulumi/kubernetes';
import * as pulumi from '@pulumi/pulumi';
import * as eksmod from '@pulumi/eksmod';
import * as aws from '@pulumi/aws';
import * as std from '@pulumi/std';
import { getKubeConfig } from './kube-config';

const azs = aws.getAvailabilityZonesOutput({
  filters: [{
        name: "opt-in-status",
        values: ["opt-in-not-required"],
    }]
}).names.apply(names => names.slice(0, 3));

const cidr = "10.0.0.0/16";

const cfg = new pulumi.Config();
const prefix = cfg.get("prefix") ?? pulumi.getStack();

const vpc = new vpcmod.Module("test-vpc", {
  azs: azs,
  name: `test-vpc-${prefix}`,
  cidr,
  private_subnets: azs.apply(azs => azs.map((_, i) => {
    return getCidrSubnet(cidr, 8, i+1);
  })),
  public_subnets: azs.apply(azs => azs.map((_, i) => {
    return getCidrSubnet(cidr, 8, i+1+4);
  })),
  intra_subnets: azs.apply(azs => azs.map((_, i) => {
    return getCidrSubnet(cidr, 8, i+1 + 8);
  })),


  enable_nat_gateway: true,
  single_nat_gateway: true,

  public_subnet_tags: {
    'kubernetes.io/role/elb': '1',
  },
  private_subnet_tags: {
    'kubernetes.io/role/internal-elb': '1',
  },
});

const cluster = new eksmod.Module('test-cluster', {
  cluster_name: `test-cluster-${prefix}`,
  cluster_endpoint_public_access: true,
  cluster_compute_config: {
    enabled: true,
    node_pools: ["general-purpose"],
  },
  vpc_id: vpc.vpc_id.apply(id => id!),
  // TODO [pulumi/pulumi-terraform-module#228] have to use a list of unknowns instead of unknown list
  subnet_ids: [
    vpc.private_subnets.apply(subnets => subnets![0]),
    vpc.private_subnets.apply(subnets => subnets![1]),
    vpc.private_subnets.apply(subnets => subnets![2]),
  ],
  enable_cluster_creator_admin_permissions: true,
}, { dependsOn: vpc });

// Make the cluster name an output so the downstream resources depend on the cluster creation
const clusterName = pulumi.all([cluster.cluster_arn, cluster.cluster_name]).apply(([_clusterArn, clusterName]) => {
  return clusterName!;
});

const kubeconfig = getKubeConfig(clusterName, {
    dependsOn: cluster,
});

const k8sProvider = new k8s.Provider("k8sProvider", {
  kubeconfig,
}, { dependsOn: cluster });


const appName = "nginx";
const ns = new k8s.core.v1.Namespace(appName, {
    metadata: { name: appName },
}, { provider: k8sProvider });

const configMap = new k8s.core.v1.ConfigMap(appName, {
    metadata: {
        namespace: ns.metadata.name,
    },
    data: {
        "index.html": "<html><body><h1>Hello, Pulumi!</h1></body></html>",
    },
}, { provider: k8sProvider });

const deployment = new k8s.apps.v1.Deployment(appName, {
    metadata: {
        namespace: ns.metadata.name
    },
    spec: {
        selector: { matchLabels: { app: appName } },
        replicas: 3,
        template: {
            metadata: { labels: { app: appName } },
            spec: {
                containers: [{
                    name: appName,
                    image: appName,
                    ports: [{ containerPort: 80 }],
                    volumeMounts: [{ name: "nginx-index", mountPath: "/usr/share/nginx/html" }],
                }],
                volumes: [{
                    name: "nginx-index",
                    configMap: { name: configMap.metadata.name },
                }],
            },
        },
    },
}, { provider: k8sProvider });

const service = new k8s.core.v1.Service(appName, {
    metadata: {
        name: appName,
        namespace: ns.metadata.name
    },
    spec: {
        selector: { app: appName },
        ports: [{ port: 80, targetPort: 80 }],
    },
}, { provider: k8sProvider, dependsOn: [deployment] });

const ingressClass = new k8s.networking.v1.IngressClass("alb", {
    metadata: {
        namespace: ns.metadata.name,
        labels: {
            "app.kubernetes.io/name": "LoadBalancerController",
        },
        name: "alb",
    },
    spec: {
        controller: "eks.amazonaws.com/alb",
    }
}, { provider: k8sProvider });

const ingress = new k8s.networking.v1.Ingress(appName, {
    metadata: {
        namespace: ns.metadata.name,
        // Annotations for EKS Auto Mode to identify the Ingress as internet-facing and target-type as IP.
        annotations: {
            "alb.ingress.kubernetes.io/scheme": "internet-facing",
            "alb.ingress.kubernetes.io/target-type": "ip",
        }
    },
    spec: {
        ingressClassName: ingressClass.metadata.name,
        rules: [{
            http: {
                paths: [{
                    path: "/",
                    pathType: "Prefix",
                    backend: {
                        service: {
                            name: service.metadata.name,
                            port: {
                                number: 80,
                            },
                        },
                    },
                }],
            },
        }],
    }
}, { provider: k8sProvider });

export const url = ingress.status.apply(status => status?.loadBalancer?.ingress?.[0]?.hostname);

function getCidrSubnet(cidr: string, newbits: number, netnum: number): pulumi.Output<string> {
    return std.cidrsubnetOutput({
    input: cidr,
    newbits,
    netnum,
  }).result
}
