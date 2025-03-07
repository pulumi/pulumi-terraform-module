# pulumi-terraform-module

This provider supports running Terraform Modules directly in Pulumi.

## Usage

To get started, run this in the context of a Pulumi program:

    pulumi package add terraform-module <module> [<version-spec>] <pulumi-package>

For example you can run the following to add the [VPC
module](https://registry.terraform.io/modules/terraform-aws-modules/vpc/aws/latest) as a Pulumi
package called "vpc":

    pulumi package add terraform-module terraform-aws-modules/vpc/aws 5.18.1 vpc

Pulumi will generate a local SDK in your current programming language and print instructions on how
to use it. For example, if your program is in TypeScript, you can start provisioning the module as
follows:

``` typescript
import * as vpc from "@pulumi/vpc";

const defaultVpc = new vpc.Module("defaultVpc", {cidr: "10.0.0.0/16"});
```

### Local Modules

Local modules are supported. Any directory with `.tf` files and optionally `variables.tf` and
`outputs.tf` is a module. It can be added to a Pulumi program with:

    pulumi package add terraform-module <path> <pulumi-package>

For example:

    pulumi package add terraform-module ./infra infra

### Configuring Terraform Providers

Some modules require Terraform providers to function. You can configure these providers from within Pulumi. For
example, when using the [terraform-aws-s3-bucket](https://github.com/terraform-aws-modules/terraform-aws-s3-bucket)
module, you can configure the `region` of the underlying provider explicitly as follows:

```typescript
import * as bucket from "@pulumi/bucket";

const provider = new bucket.Provider("test-provider", {
    aws: {
        "region": "us-west-2"
    }
})

const testBucket = new bucket.Module("test-bucket", {
    bucket: `${prefix}-test-bucket`
}, { provider: provider });
```

The relevant [Provider
Configuration](https://registry.terraform.io/providers/hashicorp/aws/latest/docs#provider-configuration) section will
be the right place to look for what keys can be configured.

Any environment variables you set for Pulumi execution will also be available to these providers. To continue with the
AWS provider example, you can ensure it can authenticate by setting `AWS_PROFILE` or else `AWS_ACCESS_KEY` and similar
environment variables.

Note that the providers powering the Module are Terraform providers and not Pulumi bridged providers such as
[pulumi-aws](https://github.com/pulumi/pulumi-aws). They are the right place to look for additional documentation.

## How it works

The modules are executed with `opentofu` binary that is automatically installed on-demand. The state is stored in your
chosen [Pulumi state backend](https://www.pulumi.com/docs/iac/concepts/state-and-backends/), defaulting to Pulumi
Cloud. [Secrets](https://www.pulumi.com/docs/iac/concepts/secrets/) are encrypted and stored securely.

## Why should I use this

You can now migrate legacy Terraform modules to Pulumi without completely rewriting their sources.

As a Pulumi user you also now have access to the mature and rich ecosystem of public Terraform modules that you can mix
and match with the rest of your Pulumi code.

## Maturity

The project is in experimental phase as we are starting to work with partners to iron out practical issues and reach
preview level of maturity. There might be some breaking changes still necessary to reach our goal of of enabling as
many Terraform modules execute seamlessly under Pulumi as possible.

Known limitations at this point include but are not limited to:

- using the `transforms` resource option
- targeted updates via `pulumi up --target ...`
- protecting individual resources deployed by the module

## Bugs


If you are having issues, we would love to hear from you as we work to make this product better:

https://github.com/pulumi/pulumi-terraform-module/issues
