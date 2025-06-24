import * as resources from "@pulumi/azure-native/resources";
import * as vnet from "@pulumi/vnet";
import * as pulumi from "@pulumi/pulumi";

const cfg = new pulumi.Config();
const rgName = cfg.get("rg") ?? pulumi.getStack();

const resourceGroup = new resources.ResourceGroup("resourceGroup", {
    resourceGroupName: rgName,
    location: "EastUS",
});

// Create a virtual network in the resource group
// requires ARM_SUBSCRIPTION_ID environment variable to be set
const virtualNetwork = new vnet.Module("testvnet", {
    resource_group_name: resourceGroup.name,
    location: resourceGroup.location,
    address_space: ["10.0.0.0/16"],
})

export const networkId = virtualNetwork.name;