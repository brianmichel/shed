defmodule Garden.SandboxBackend do
  @moduledoc """
  Pluggable runtime backend for sandbox execution.

  Configure with:

      config :garden, :sandbox_backend, Garden.SandboxBackend.LocalHost

  Built-ins:
  - `Garden.SandboxBackend.Mock`
  - `Garden.SandboxBackend.LocalHost`
  """

  @callback name() :: String.t()
  @callback setup_sandbox(map(), pid()) :: :ok | {:ok, pid()} | {:error, term()}
  @callback teardown_sandbox(String.t()) :: :ok
  @callback start_command(String.t(), map()) :: :ok | {:error, term()}
  @callback send_stdin(String.t(), String.t(), String.t()) :: :ok | {:error, term()}
  @callback cancel_command(String.t(), String.t(), map()) :: :ok | {:error, term()}
  @callback kill_command(String.t(), String.t()) :: :ok | {:error, term()}
  @callback list_files(map(), String.t()) :: {:ok, %{path: String.t(), entries: list(String.t())}} | {:error, term()}
  @callback read_file(map(), String.t()) :: {:ok, %{path: String.t(), content: String.t()}} | {:error, term()}
  @callback write_file(map(), String.t(), String.t()) :: {:ok, %{path: String.t(), bytes: non_neg_integer()}} | {:error, term()}
  @callback workspace_root(map()) :: String.t()

  def implementation do
    Application.get_env(:garden, :sandbox_backend, Garden.SandboxBackend.Mock)
  end

  def name, do: implementation().name()
  def setup_sandbox(sandbox, store), do: implementation().setup_sandbox(sandbox, store)
  def teardown_sandbox(id), do: implementation().teardown_sandbox(id)
  def start_command(id, command), do: implementation().start_command(id, command)
  def send_stdin(id, cmd_id, data), do: implementation().send_stdin(id, cmd_id, data)
  def cancel_command(id, cmd_id, attrs), do: implementation().cancel_command(id, cmd_id, attrs)
  def kill_command(id, cmd_id), do: implementation().kill_command(id, cmd_id)
  def list_files(sandbox, path), do: implementation().list_files(sandbox, path)
  def read_file(sandbox, path), do: implementation().read_file(sandbox, path)
  def write_file(sandbox, path, content), do: implementation().write_file(sandbox, path, content)
  def workspace_root(sandbox), do: implementation().workspace_root(sandbox)
end
