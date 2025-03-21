import * as albmod from '@pulumi/albmod';
import * as bucketmod from '@pulumi/bucketmod';
import * as lambdamod from '@pulumi/lambdamod';
import * as vpcmod from '@pulumi/vpcmod';
import * as pulumi from '@pulumi/pulumi';
import * as aws from '@pulumi/aws';
import * as std from '@pulumi/std';
import { join } from 'path';

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
  public_subnets: azs.apply(azs => azs.map((_, i) => {
    return getCidrSubnet(cidr, i+1);
  })),
  private_subnets: azs.apply(azs => azs.map((_, i) => {
    return getCidrSubnet(cidr, i+1+4);
  })),

  enable_nat_gateway: true,
  single_nat_gateway: true,
});

const logBucket = new bucketmod.Module('test-log-bucket', {
  bucket_prefix: `test-log-bucket-${prefix}`,
  force_destroy: true,
  control_object_ownership: true,
  attach_lb_log_delivery_policy: true, // required for ALB logs
  attach_elb_log_delivery_policy: true,
  attach_access_log_delivery_policy: true,
  attach_deny_insecure_transport_policy: true,
  attach_require_latest_tls_policy: true,
});

const lambda = new lambdamod.Module('test-lambda', {
  function_name: `test-lambda-${prefix}`,
  source_path: join(__dirname, 'app', 'index.ts'),
  runtime: "nodejs22.x",
  publish: true,
  handler: "index.handler",
});

new albmod.Module('test-alb', {
  enable_deletion_protection: false, // for example only
  vpc_id: vpc.vpc_id.apply(id => id!),
  subnets: [
    vpc.public_subnets[0],
    vpc.public_subnets[1],
    vpc.public_subnets[2],
  ],
  security_group_ingress_rules: {
    all_http: {
      from_port: 80,
      to_port: 80,
      ip_protocol: "tcp",
      description: "HTTP web traffic",
      cidr_ipv4: "0.0.0.0/0",
    }
  },
  security_group_egress_rules: {
    all: {
      ip_protocol: "-1",
      cidr_ipv4: vpc.vpc_cidr_block.apply(cidr => cidr!),
    }
  },
  listeners: {
    http: {
      protocol: "HTTP",
      forward: {
        target_group_key: "lambda-without-trigger",
      }
    }
  },
  target_groups: {
    'lambda-without-trigger': {
      name_prefix: 'l1-',
      target_type: 'lambda',
      target_id: lambda.lambda_function_arn.apply(arn => arn!),
      attach_lambda_permission: true,
    },
  },
  access_logs: {
    bucket: logBucket.s3_bucket_id.apply(id => id!),
    prefix: 'access-logs',
  },
}, { dependsOn: [vpc, logBucket, lambda] });

function getCidrSubnet(cidr: string, netnum: number): pulumi.Output<string> {
    return std.cidrsubnetOutput({
    input: cidr,
    newbits: 8,
    netnum,
  }).result
}
