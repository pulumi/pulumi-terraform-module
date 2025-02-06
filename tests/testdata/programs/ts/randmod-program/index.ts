import * as randmod from "@pulumi/randmod";

const m = new randmod.Module("myrandmod", {
    maxlen: 10, // why is this not a number?
});

export const randomPriority = m.random_priority;
