defmodule Garden.Sandboxes do
  @moduledoc """
  In-memory sandbox and command orchestration used to flesh out the Garden API.

  This context currently uses a mock compute runtime backed by GenServers so the
  HTTP layer and future client SDKs can be exercised before real container/VM
  orchestration exists.
  """

  alias Garden.Sandboxes.Store

  def sandbox_topic(sandbox_id), do: "sandboxes:" <> sandbox_id
  def command_topic(sandbox_id, command_id), do: "sandboxes:" <> sandbox_id <> ":commands:" <> command_id

  def list_sandboxes do
    Store.list_sandboxes()
  end

  def acquire_sandbox(attrs) do
    Store.acquire_sandbox(attrs)
  end

  def ensure_sandbox(id, attrs \\ %{}) do
    Store.ensure_sandbox(id, attrs)
  end

  def get_sandbox(id) do
    Store.get_sandbox(id)
  end

  def extend_lease(id, attrs) do
    Store.extend_lease(id, attrs)
  end

  def release_sandbox(id, attrs \\ %{}) do
    Store.release_sandbox(id, attrs)
  end

  def list_sandbox_events(id, opts \\ %{}) do
    Store.list_sandbox_events(id, opts)
  end

  def list_commands(sandbox_id) do
    Store.list_commands(sandbox_id)
  end

  def start_command(sandbox_id, attrs) do
    Store.start_command(sandbox_id, attrs)
  end

  def get_command(sandbox_id, command_id) do
    Store.get_command(sandbox_id, command_id)
  end

  def send_stdin(sandbox_id, command_id, attrs) do
    Store.send_stdin(sandbox_id, command_id, attrs)
  end

  def cancel_command(sandbox_id, command_id, attrs \\ %{}) do
    Store.cancel_command(sandbox_id, command_id, attrs)
  end

  def kill_command(sandbox_id, command_id, attrs \\ %{}) do
    Store.kill_command(sandbox_id, command_id, attrs)
  end

  def list_command_events(sandbox_id, command_id, opts \\ %{}) do
    Store.list_command_events(sandbox_id, command_id, opts)
  end

  def list_files(sandbox_id, path \\ "/workspace") do
    Store.list_files(sandbox_id, path)
  end

  def read_file(sandbox_id, path) do
    Store.read_file(sandbox_id, path)
  end

  def write_file(sandbox_id, path, content) do
    Store.write_file(sandbox_id, path, content)
  end
end
