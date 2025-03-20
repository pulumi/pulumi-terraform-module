# AWS EKS Module Example

## Setup Steps

Install the Terraform modules using `pulumi package add`.

```console
$ yarn install
$ pulumi package add terraform-module terraform-aws-modules/vpc/aws 5.19.0 vpcmod
$ pulumi package add terraform-module terraform-aws-modules/eks/aws 20.34.0 eksmod
```

## Deploy

To deploy the example run

```console
$ pulumi up
```
