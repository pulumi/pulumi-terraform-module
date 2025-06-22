import pulumi
import pulumi_dashed as dashed

test = dashed.Module("test-bucket", dashed_input="example")
pulumi.export('result', test.dashed_output)
