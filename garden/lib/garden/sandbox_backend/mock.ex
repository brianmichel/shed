defmodule Garden.SandboxBackend.Mock do
  @behaviour Garden.SandboxBackend

  alias Garden.Sandboxes.MockCompute
  alias Garden.Sandboxes.MockComputeSupervisor

  @impl true
  def name, do: "mock"

  @impl true
  def setup_sandbox(sandbox, store) do
    ensure_supervisor()
    case :global.whereis_name({MockCompute, sandbox.id}) do
      :undefined -> MockComputeSupervisor.start_compute(sandbox.id, store)
      _pid -> :ok
    end
  end

  @impl true
  def teardown_sandbox(sandbox_id), do: MockCompute.terminate_compute(sandbox_id)
  @impl true
  def start_command(sandbox_id, command), do: MockCompute.start_command(sandbox_id, command)
  @impl true
  def send_stdin(sandbox_id, command_id, data), do: MockCompute.send_stdin(sandbox_id, command_id, data)
  @impl true
  def cancel_command(sandbox_id, command_id, attrs), do: MockCompute.cancel_command(sandbox_id, command_id, attrs)
  @impl true
  def kill_command(sandbox_id, command_id), do: MockCompute.kill_command(sandbox_id, command_id)

  @impl true
  def list_files(sandbox, path), do: Garden.SandboxBackend.LocalHost.list_files(sandbox, path)
  @impl true
  def read_file(sandbox, path), do: Garden.SandboxBackend.LocalHost.read_file(sandbox, path)
  @impl true
  def write_file(sandbox, path, content), do: Garden.SandboxBackend.LocalHost.write_file(sandbox, path, content)
  @impl true
  def workspace_root(sandbox), do: Garden.SandboxBackend.LocalHost.workspace_root(sandbox)

  defp ensure_supervisor do
    case Process.whereis(MockComputeSupervisor) do
      nil -> MockComputeSupervisor.start_link([])
      _ -> :ok
    end
    :ok
  end
end
