package "vpc" {
    baseProviderName = "terraform-module"
    baseProviderVersion = "0.0.1"
    parameterization {
        name = "vpc"
        version = "5.18.1"
        value = "eyJtb2R1bGUiOiJ0ZXJyYWZvcm0tYXdzLW1vZHVsZXMvdnBjL2F3cyIsInZlcnNpb24iOiI1LjE4LjEiLCJwYWNrYWdlTmFtZSI6InZwYyJ9"
    }
}

resource "defaultVpc" "vpc:index:Module" {
    cidr = "10.0.0.0/16"
}

output "vpcId" {
    value = defaultVpc.vpc_id
}
