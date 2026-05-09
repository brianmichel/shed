defmodule Garden.SeedSimulatorTest do
  use ExUnit.Case, async: false

  alias Garden.Sandboxes
  alias Garden.Sandboxes.Store, as: SandboxStore
  alias Garden.SeedSessions
  alias Garden.SeedSessions.Store
  alias Garden.SeedSimulator

  setup do
    Store.reset!()
    SandboxStore.reset!()
    :ok
  end

  test "simulator drives command lifecycle through seed protocol" do
    {:ok, sandbox} = Sandboxes.ensure_sandbox("sbx_sim_test", %{"environment" => "linux", "template" => "default", "ttl_ms" => 1_000})

    {:ok, session} = SeedSessions.issue_session(sandbox.id)
    {:ok, _pid} = SeedSimulator.start_for_session(session.session_id)
    Process.sleep(60)

    {:ok, command} = Sandboxes.start_command(sandbox.id, %{"command" => "echo hello", "stdin" => true})
    Process.sleep(120)

    assert {:ok, finished} = Sandboxes.get_command(sandbox.id, command.id)
    assert finished.state == "exited"

    assert {:ok, %{events: events}} = Sandboxes.list_command_events(sandbox.id, command.id, %{})
    assert Enum.any?(events, &(&1.type == "command.started"))
    assert Enum.any?(events, &(&1.type == "command.stdout"))
    assert Enum.any?(events, &(&1.type == "command.exit"))
  end
end
