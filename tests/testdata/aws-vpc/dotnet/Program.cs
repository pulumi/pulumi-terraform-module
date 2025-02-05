using System.Collections.Generic;
using System.Linq;
using Pulumi;
using TerraformAwsModules = Pulumi.TerraformAwsModules;

return await Deployment.RunAsync(() => 
{
    var vpc = new TerraformAwsModules.Vpc("vpc", new()
    {
        Cidr = "10.0.0.0/16",
    });

    return new Dictionary<string, object?>
    {
        ["vpcId"] = vpc.Vpc_id,
    };
});

