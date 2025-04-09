# Resource Deletion

Resource deletion can vary depending on under which scenario deletion is
occurring. At a high level a resource can be deleted:

- `RegisterResource` it not called for an existing resource.
- `pulumi dn` is called to destroy the resource
- During `pulumi up` `Diff` is called and reports a resource replacement. The
  replacement can either have:
  - DeleteBeforeReplace=true
  - DeleteBeforeReplace=false

## RegisterResource not called for existing resource

In this case there is a resource that was previously created by the TF Module
that is now not being created. As part of the `ModuleComponent` the `TF Apply`
will be run and the updated state will be returned which will not contain the
removed resource. The ModuleComponent will then not emit a `RegisterResource`
call which will cause the Pulumi engine to call `Delete` on the child resource.

- The `Delete` call will happen in a different provider process so it will not
  have access to any of the shared state.
- If the `TF Apply` fails for any reason, then the child resource `Delete` will
  never be called. The resource will be deleted from TF, but not from Pulumi.

The `TestPartialDestroyFromRemovalOnUpdate` test illustrates this case.

## Pulumi destroy is called

In this case `Delete` is called for both the `ModuleState` resource and the
child resources and all the `Delete` calls share the same provider process which
means they can share provider state.

The `TestPartialDestroy` test illustrates this case.

## Replacement with DeleteBeforeReplace=false

In this case the `TF Plan` returns a plan which includes some resources that are
being replaced. `DeleteBeforeReplace=false` is the default from TF. The child
resource `Diff` method is called which returns a replacement diff. The Pulumi
engine then calls `Create` and then `Delete` on the child resource. Both these
calls share the same provider process that is already running, so they are able
to access the provider state.

The `TestPartialDestroyOnUpdate` test illustrates this case.

## Replacement with DeleteBeforeReplace=true

In this case the `TF Plan` returns a plan which includes some resources that are
being replaced. `DeleteBeforeReplace=true` is not the default from TF so this
might not occur very often. The child resource `Diff` method is called which
returns a replacement diff. The Pulumi engine then calls `Delete` and then
`Replace` on the child resource. The `Delete` call occurs in a separate provider
process so it does not have access to the shared provider state.

The `Test_replace_trigger_delete_create` test illustrates this case.
