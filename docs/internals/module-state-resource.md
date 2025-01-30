# ModuleState Resource

The ModuleState Custom Resource stores state for its parent Component Resource.

Currently Pulumi does not naturally allow Component Resources to have state of their own. ModuleState uses a trick to
work around this:

```
    ModuleResource (ComponentResource)
       |
       +-- ModuleStateResource (CustomResource)

```

Pulumi CLI calls `ModuleResource.Construct()` during `pulumi preview` and `pulumi up`. The trick allows construct to read
and write a state:

``` go
func (*m) Construct(...) {
   state := getState()
   defer saveState(state)
   constructWithState(&state)
}
```

Specifically, the state is read when Pulumi CLI calls Create or Diff on ModuleStateResource, and it is written back when
the ModuleStateResource responds with the updated state.

There are Create, Update/Unchagned, and Update/Changed scenarios. The following diagram shows what happens in the
Update/Changed scenario which is the most complicated one of the three.

``` mermaid

sequenceDiagram
  participant PulumiCLI
  participant ModRes
  participant StateRes

  PulumiCLI->>ModRes:  Construct()
  ModRes->>PulumiCLI:  RegisterResource(StateRes)

  PulumiCLI->>StateRes: Check()
  PulumiCLI->>StateRes: Diff(oldState=o)

  StateRes->>ModRes:    oldStatePromise.Fulfill(o) // in-memory

  ModRes->>PulumiCLI:   RegisterResource(children...)
  ModRes->>StateRes:    newStatePromise.Fulfill(n) // in-memory

  ModRes->>PulumiCLI:   RegisterResourceOutputs()
  ModRes->>PulumiCLI:   ConstructResult(..)

  StateRes->>PulumiCLI: DiffResult(o, n)
  PulumiCLI->>StateRes: Update(...)

```

The state read/write is exposed to Pulumi CLI lifecycle methods on the ModuleStateResource (abbreviated StateRes in the
diagram), but use an in-memory side channel to expose the state to the ModuleResoure (abbreviated ModRes).

For this to work without blocking, ModuleStateResource lifecycle methods have to happen in the same OS process as the
ModuleResource, and they have to be performed concurrently with `Construct()`, in a dedicated goroutine. Attempting to
perform them sequentially would deadlock.
