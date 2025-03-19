import * as pulumi from '@pulumi/pulumi';
import * as aws from '@pulumi/aws';
function getRegion(opts?: pulumi.InvokeOptions): pulumi.Output<string> {
    return pulumi.output(aws.getRegionOutput({}, opts ).name);
}
export function getKubeConfig(
  clusterName: pulumi.Input<string>,
  opts?: pulumi.InvokeOutputOptions,
): pulumi.Output<string> {
  const cluster = aws.eks.getClusterOutput(
    {
      name: clusterName,
    },
  );

  return pulumi.output(
    cluster.certificateAuthorities.apply((certificateAuthorities) => {
      return pulumi.jsonStringify({
        apiVersion: 'v1',
        kind: 'Config',
        clusters: [
          {
            name: cluster.arn,
            cluster: {
              server: cluster.endpoint,
              'certificate-authority-data': certificateAuthorities[0].data,
            },
          },
        ],
        contexts: [
          {
            name: cluster.arn,
            context: {
              user: cluster.arn,
              cluster: cluster.arn,
            },
          },
        ],
        'current-context': cluster.arn,
        users: [
          {
            name: cluster.arn,
            user: {
              exec: {
                apiVersion: 'client.authentication.k8s.io/v1beta1',
                args: [
                  '--region',
                  getRegion(opts),
                  'eks',
                  'get-token',
                  '--cluster-name',
                  clusterName,
                  '--output',
                  'json',
                ],
                command: 'aws',
              },
            },
          },
        ],
      });
    }),
  );
}
