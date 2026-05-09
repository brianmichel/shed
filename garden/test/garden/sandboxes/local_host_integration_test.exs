defmodule Garden.Sandboxes.LocalHostIntegrationTest do
  use ExUnit.Case, async: false

  alias Garden.Sandboxes
  alias Garden.Sandboxes.Store
  alias Garden.SandboxBackend

  setup do
    Store.reset!()

    # Switch to LocalHost backend for this test
    original = Application.get_env(:garden, :sandbox_backend)
    Application.put_env(:garden, :sandbox_backend, Garden.SandboxBackend.LocalHost)

    on_exit(fn ->
      Application.put_env(:garden, :sandbox_backend, original)
    end)

    :ok
  end

  test "full flow with LocalHost backend: ensure sandbox, run command, get events" do
    sandbox_id = "sbx_local_host_test"

    # Ensure sandbox (this triggers setup_sandbox -> LocalHostRuntime.ensure_started)
    {:ok, sandbox} = Sandboxes.ensure_sandbox(sandbox_id, %{"state" => "ready"})
    assert sandbox.state == "ready"
    assert SandboxBackend.name() == "local_host"

    # Start a command through the API
    {:ok, command} = Sandboxes.start_command(sandbox_id, %{"command" => "echo hello", "stdin" => false})
    assert command.state == "queued"

    # Give the command time to run
    Process.sleep(500)

    # Check the command status
    {:ok, updated} = Sandboxes.get_command(sandbox_id, command.id)
    IO.inspect(updated, label: "Command after run")

    # Get events
    {:ok, %{events: events}} = Sandboxes.list_command_events(sandbox_id, command.id, %{})
    IO.inspect(events, label: "Events from LocalHost integration test")

    assert Enum.any?(events, &(&1.type == "command.queued")),
           "Expected command.queued"
    assert Enum.any?(events, &(&1.type == "command.started")),
           "Expected command.started but got: #{inspect(Enum.map(events, & &1.type))}"
    assert Enum.any?(events, &(&1.type == "command.stdout")),
           "Expected command.stdout but got: #{inspect(Enum.map(events, & &1.type))}"
    assert Enum.any?(events, &(&1.type == "command.exit")),
           "Expected command.exit but got: #{inspect(Enum.map(events, & &1.type))}"

    stdout_chunks =
      events
      |> Enum.filter(&(&1.type == "command.stdout"))
      |> Enum.map(& &1.data["chunk"])

    assert "hello\n" in stdout_chunks,
           "Expected 'hello\\n' in stdout chunks: #{inspect(stdout_chunks)}"
  end
end
