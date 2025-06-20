using System.Collections.Generic;
using Pulumi;

return await Deployment.RunAsync(() =>
{
    var test = new Pulumi.Dashed.Module("test", new()
    {
        Dashed_input = "example"
    });

    return new Dictionary<string, object?>
    {
        ["result"] = test.Dashed_output
    };
});
