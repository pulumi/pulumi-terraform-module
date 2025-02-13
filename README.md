# pulumi-terraform-module

EXPERIMENTAL support for running Terraform Modules directly in Pulumi.

## Usage

To get started, run this in the context of a Pulumi program:

    pulumi package add <module> [<version-spec>] <pulumi-package>

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

    pulumi package add <path> <pulumi-package>

For example:

    pulumi package add ./infra infra
