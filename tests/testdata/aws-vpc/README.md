### Description 

This PCL file specifies an example program that uses a parameterized terraform module to create a VPC.

The idea is that you are able to `pulumi convert` the program into multiple languages and it will generate the SDK, add its reference to the program in that language.

The `package` block specifies how the terraform module provider is paramterized. The value of the parameter contains the tf module we are working with as well as its version. The parameter value is a base64 encoded string of a JSON string that looks like this:
```json
{ 
    "module": "terraform-aws-modules/vpc/aws",
    "version": "5.18.1",
    "packageName": "vpc"
}
```
To generate the parameter value, you must first create the JSON string and then base64 encode it. You can do that using `dotnet fsi` which runs F# interactive, then run the following:
```fs
dict [ 
    "module", "terraform-aws-modules/vpc/aws"
    "version", "5.18.1"
    "packageName", "vpc"
]
|> System.Text.Json.JsonSerializer.Serialize
|> System.Text.Encoding.UTF8.GetBytes
|> System.Convert.ToBase64String
```