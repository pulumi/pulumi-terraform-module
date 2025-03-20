import * as pulumi from '@pulumi/pulumi';
import * as randmod from "@pulumi/randmod";
import * as random from '@pulumi/random';

const seed = new random.RandomInteger('seed', {
    max: 16,
    min: 1,
    seed: 'the-most-random-seed',
});

// Expect Pulumi to recognize that randmod.Module depends on RandomInteger.
const m = new randmod.Module("myrandmod", {
    maxlen: 10,
    randseed: pulumi.secret(seed.result.apply(n => String(n))),
});

export const randomPriority = m.random_priority;
export const randomSeed = m.random_seed;
