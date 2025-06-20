import * as pulumi from "@pulumi/pulumi";
import * as dashed from "@pulumi/dashed";

const test = new dashed.Module("test", {
    dashed_input: "example",
});

export const result = test.dashed_output;