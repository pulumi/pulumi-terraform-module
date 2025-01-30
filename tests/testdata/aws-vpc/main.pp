package "terraform-aws-modules" {
    baseProviderName = "terraform-module-provider"
    baseProviderVersion = "0.1.0"
    parameterization {
        name = "terraform-aws-modules"
        version = "5.18.1"
        value = "eyJtb2R1bGUiOiJ0ZXJyYWZvcm0tYXdzLW1vZHVsZXMvdnBjL2F3cyIsInZlcnNpb24iOiI1LjE4LjEifQ=="
    }
}

resource "vpc" "terraform-aws-modules:index:Vpc" {
    cidr = "10.0.0.0/16"
}

output "vpcId" {
    value = vpc.vpc_id
}