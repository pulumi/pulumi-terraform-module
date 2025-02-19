import * as randmod from "@pulumi/randmod";
import * as random from '@pulumi/random';

const seed = new random.RandomInteger('seed', {
    max: 16,
    min: 1,
    seed: 'the-most-random-seed',
});

const m = new randmod.Module("myrandmod", {
    maxlen: 10,
    randseed: seed.result.apply(r => r.toString()),
});

export const randomPriority = m.random_priority;
