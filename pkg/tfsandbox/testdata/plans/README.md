# Plan Data

These test plans are generated using an example module.

1. Create a `main.tf` file

```console
$ mkdir terraform-test-data
$ cd terraform-test-data && touch main.tf
```

```hcl
module "s3_bucket" {
  source = "terraform-aws-modules/s3-bucket/aws"

  acl = "private"
  versioning = {
    enabled = true
  }

  control_object_ownership = true
  object_ownership         = "ObjectWriter"
}


```

2. Run plan & output plan to file

```console
$ terraform plan -out=tfplan
$ terraform show -json tfplan | jq '.' > plan_data.json
```

3. The data can then be manually sanitized by replacing sensitive values (like
account ids).
