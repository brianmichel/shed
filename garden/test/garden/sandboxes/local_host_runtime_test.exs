defmodule Garden.Sandboxes.LocalHostRuntimeTest do
  use ExUnit.Case, async: false

  alias Garden.Sandboxes.LocalHostRuntime
  alias Garden.Sandboxes.LocalHostRuntimeSupervisor
  alias Garden.Sandboxes.Store

  setup do
    Store.reset!()
    :ok
  end

  test "runs a command and produces output events" do
    sandbox_id = "sbx_test_" <> Base.encode16(:crypto.strong_rand_bytes(4), case: :lower)
    store = Process.whereis(Store)
    root = Path.join(System.tmp_dir!(), sandbox_id)
    File.mkdir_p!(root)

    sandbox = %{
      id: sandbox_id,
      environment: "linux",
      template: "default",
      state: "ready",
      metadata: %{},
      capabilities: %{"commands" => true},
      lease: %{"ttl_ms" => 30_000, "expires_at" => DateTime.to_iso8601(DateTime.utc_now())},
      inserted_at: DateTime.utc_now(),
      updated_at: DateTime.utc_now()
    }

    # Ensure supervisor is running
    case LocalHostRuntimeSupervisor.start_link([]) do
      {:ok, _sup} -> :ok
      {:error, {:already_started, _}} -> :ok
    end

    {:ok, _pid} = LocalHostRuntimeSupervisor.start_runtime(sandbox_id, store, root, sandbox)

    # Register sandbox and command in store so list_command_events works
    :sys.replace_state(store, fn state ->
      command = %{
        id: "cmd_test_1",
        sandbox_id: sandbox_id,
        state: "queued",
        command: "echo hello",
        cwd: "/workspace",
        env: %{},
        stdin: false,
        timeout_ms: 60_000,
        metadata: %{},
        pid: nil,
        exit_code: nil,
        signal: nil,
        started_at: nil,
        completed_at: nil,
        inserted_at: DateTime.utc_now(),
        updated_at: DateTime.utc_now()
      }

      state
      |> put_in([:sandboxes, sandbox_id], sandbox)
      |> put_in([:commands, sandbox_id], %{"cmd_test_1" => command})
    end)

    # Create a command map matching what the store sends
    command = %{
      id: "cmd_test_1",
      sandbox_id: sandbox_id,
      command: "echo hello",
      cwd: "/workspace",
      env: %{},
      stdin: false,
      timeout_ms: 60_000,
      metadata: %{}
    }

    # Start the command
    :ok = LocalHostRuntime.start_command(sandbox_id, command)

    # Give it time to run
    Process.sleep(500)

    # Check events in store
    {:ok, %{events: events}} = Store.list_command_events(sandbox_id, command.id, %{})

    IO.inspect(events, label: "Events captured")

    assert Enum.any?(events, &(&1.type == "command.started")),
           "Expected command.started event but got: #{inspect(Enum.map(events, & &1.type))}"

    assert Enum.any?(events, &(&1.type == "command.stdout")),
           "Expected command.stdout event but got: #{inspect(Enum.map(events, & &1.type))}"

    assert Enum.any?(events, &(&1.type == "command.exit")),
           "Expected command.exit event but got: #{inspect(Enum.map(events, & &1.type))}"

    stdout_event = Enum.find(events, &(&1.type == "command.stdout"))
    assert stdout_event.data["chunk"] == "hello\n"
  end
end
