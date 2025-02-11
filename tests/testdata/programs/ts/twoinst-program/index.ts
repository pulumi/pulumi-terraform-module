import * as randmod from "@pulumi/randmod";

const m1 = new randmod.Module("myrandmod1", {
    maxlen: 10,
});

const m2 = new randmod.Module("myrandmod2", {
    maxlen: 10,
});

export const randomPriority1 = m1.random_priority;
export const randomPriority2 = m2.random_priority;
