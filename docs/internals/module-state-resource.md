# moduleStateResource

The moduleStateResource Custom Resource stores state for its parent Component which otherwise would
be stateless.

Currently Pulumi does not naturally allow Component Resources to have state of their own. Therefore
moduleStateResource uses a trick to work around this:

```
    ModuleComponentResource
       |
       +-- moduleStateResource (CustomResource)

```

Pulumi CLI calls `ModuleComponentResource.Construct()` during `pulumi preview` and `pulumi up`. The
trick allows construct to read and write a state:

``` go
func (*m) Construct(...) {
   state := getState()
   defer saveState(state)
   constructWithState(&state)
}
```

Specifically, the state is read when Pulumi CLI calls Create or Diff on the moduleStateResource, and
it is written back when the moduleStateResource responds with the updated state.

There are Create, Update/Unchagned, and Update/Changed scenarios. The following diagram shows what
happens in the Update/Changed scenario which is the most complicated one of the three.

``` mermaid

sequenceDiagram
  participant PulumiCLI
  participant ModuleComponentResource
  participant moduleStateResource

  PulumiCLI->>ModuleComponentResource:  Construct()
  ModuleComponentResource->>PulumiCLI:  RegisterResource(moduleStateResource)

  PulumiCLI->>moduleStateResource: Check()
  PulumiCLI->>moduleStateResource: Diff(oldState=o)

  moduleStateResource->>ModuleComponentResource:    oldStatePromise.Fulfill(o) // in-memory

  ModuleComponentResource->>PulumiCLI:   RegisterResource(children...)
  ModuleComponentResource->>moduleStateResource:    newStatePromise.Fulfill(n) // in-memory

  ModuleComponentResource->>PulumiCLI:   RegisterResourceOutputs()
  ModuleComponentResource->>PulumiCLI:   ConstructResult(..)

  moduleStateResource->>PulumiCLI: DiffResult(o, n)
  PulumiCLI->>moduleStateResource: Update(...)

```

The state read/write is exposed to Pulumi CLI lifecycle methods on the moduleStateResource, but use
an in-memory side channel to expose the state to the ModuleComponentResource.

For this to work without blocking, moduleStateResource lifecycle methods have to happen in the same
OS process as the ModuleComponentResource, and they have to be performed concurrently with
`Construct()`, in a dedicated goroutine. Attempting to perform them sequentially would deadlock.
