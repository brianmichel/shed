defmodule Garden.Sandboxes.LocalHostRuntimeSupervisor do
  use DynamicSupervisor

  def start_link(arg \\ []) do
    DynamicSupervisor.start_link(__MODULE__, arg, name: __MODULE__)
  end

  def init(_arg), do: DynamicSupervisor.init(strategy: :one_for_one)

  def start_runtime(sandbox_id, store, root, sandbox) do
    spec = {Garden.Sandboxes.LocalHostRuntime, sandbox_id: sandbox_id, store: store, root: root, sandbox: sandbox}
    DynamicSupervisor.start_child(__MODULE__, spec)
  end
end
