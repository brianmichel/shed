defmodule Garden.SeedSimulator do
  @moduledoc """
  In-process mock Seed client used for exercising the Garden ↔ Seed protocol.
  """

  use GenServer

  alias Garden.SeedProtocol.Message
  alias Garden.SeedSessions

  def start_link(opts) do
    session_id = Keyword.fetch!(opts, :session_id)
    GenServer.start_link(__MODULE__, opts, name: via(session_id))
  end

  def start_for_session(session_id) do
    DynamicSupervisor.start_child(Garden.SeedSimulatorSupervisor, {__MODULE__, session_id: session_id})
  end

  def init(opts) do
    session_id = Keyword.fetch!(opts, :session_id)
    {:ok, session} = SeedSessions.get(session_id)
    Phoenix.PubSub.subscribe(Garden.PubSub, SeedSessions.topic(session_id))
    {:ok, _} = SeedSessions.connect(session_id, self())

    state = %{
      session_id: session_id,
      sandbox_id: session.sandbox_id,
      seed_id: "seed-sim-" <> String.slice(session_id, -6, 6),
      seq: 0,
      generation: 1,
      commands: %{}
    }

    send(self(), :bootstrap)
    {:ok, state}
  end

  def handle_info(:bootstrap, state) do
    state = emit(state, "seed.hello", %{
      "seed_version" => "0.1.0-sim",
      "protocol_version" => "1",
      "platform" => "linux",
      "arch" => "amd64",
      "hostname" => "seed-simulator",
      "session_key" => "simulated"
    })

    state = emit(state, "seed.register", %{
      "seed_id" => state.seed_id,
      "seed_version" => "0.1.0-sim",
      "platform" => "linux",
      "arch" => "amd64",
      "hostname" => "seed-simulator",
      "boot_time" => DateTime.to_iso8601(DateTime.utc_now()),
      "workspace_root" => "/workspace",
      "process_id" => 1
    })

    state = emit(state, "seed.capabilities", %{
      "commands" => true,
      "stdin" => true,
      "cancel" => true,
      "kill" => true,
      "files" => %{"read" => true, "write" => true, "edit" => true, "search" => true},
      "pty" => false,
      "metrics" => true,
      "ports" => false,
      "snapshots" => false
    })

    state = emit(state, "seed.status", %{
      "state" => "ready",
      "hostname" => "seed-simulator",
      "platform" => "linux",
      "arch" => "amd64",
      "seed_version" => "0.1.0-sim",
      "uptime_ms" => 100,
      "workspace_root" => "/workspace",
      "active_commands" => 0,
      "network" => %{"reachable" => true}
    })

    Process.send_after(self(), :heartbeat, 5_000)
    {:noreply, state}
  end

  def handle_info(:heartbeat, state) do
    state = emit(state, "seed.heartbeat", %{
      "uptime_ms" => System.monotonic_time(:millisecond),
      "active_commands" => map_size(state.commands),
      "last_garden_seq_seen" => garden_seq(state),
      "last_seed_seq_sent" => state.seq,
      "connection_generation" => state.generation
    })

    state = emit(state, "seed.metrics", %{
      "cpu_percent" => 0.5,
      "memory_total_bytes" => 1_073_741_824,
      "memory_used_bytes" => 67_108_864,
      "disk_total_bytes" => 10_737_418_240,
      "disk_used_bytes" => 536_870_912,
      "fs_available_bytes" => 10_200_547_328,
      "rx_bytes" => 1024,
      "tx_bytes" => 2048,
      "sampled_at" => DateTime.to_iso8601(DateTime.utc_now())
    })

    Process.send_after(self(), :heartbeat, 5_000)
    {:noreply, state}
  end

  def handle_info({:seed_message, _message}, state), do: {:noreply, state}

  def handle_info({:garden_message, message}, state) do
    state = maybe_ack_garden_message(state, message)

    case message do
      %{"type" => "garden.hello"} ->
        state = emit(state, "seed.status", %{
          "state" => "ready",
          "hostname" => "seed-simulator",
          "platform" => "linux",
          "arch" => "amd64",
          "seed_version" => "0.1.0-sim",
          "uptime_ms" => 100,
          "workspace_root" => "/workspace",
          "active_commands" => map_size(state.commands),
          "network" => %{"reachable" => true}
        })

        {:noreply, state}

      %{"type" => "garden.shutdown", "payload" => payload} ->
        state = emit(state, "seed.goodbye", %{
          "reason" => payload["reason"] || "shutdown",
          "final_command_count" => map_size(state.commands),
          "sent_at" => DateTime.to_iso8601(DateTime.utc_now())
        })

        {:stop, :normal, state}

      %{"type" => "garden.drain"} ->
        state = emit(state, "seed.draining", %{"active_commands" => map_size(state.commands)})
        {:noreply, state}

      %{"type" => "command.start", "payload" => payload} ->
        handle_command_start(state, payload)

      %{"type" => "command.stdin", "payload" => payload} ->
        handle_command_stdin(state, payload)

      %{"type" => "command.cancel", "payload" => payload} ->
        handle_command_cancel(state, payload)

      %{"type" => "command.kill", "payload" => payload} ->
        handle_command_kill(state, payload)

      _ ->
        {:noreply, state}
    end
  end

  defp handle_command_start(state, payload) do
    command_id = payload["command_id"]
    command = %{"command" => payload["command"], "cancelled" => false}
    state = %{state | commands: Map.put(state.commands, command_id, command)}

    state = emit(state, "command.accepted", %{"command_id" => command_id, "state" => "queued"})
    Process.send_after(self(), {:command_started, command_id}, 25)
    {:noreply, state}
  end

  defp handle_command_stdin(state, payload) do
    command_id = payload["command_id"]
    data = payload["data"] || ""
    state = emit(state, "command.stdin.accepted", %{"command_id" => command_id, "bytes" => byte_size(data)})
    state = emit(state, "command.stdout", %{"command_id" => command_id, "chunk" => "stdin> #{data}", "encoding" => "utf-8", "stream_seq" => next_stream_seq(state, command_id)})
    {:noreply, state}
  end

  defp handle_command_cancel(state, payload) do
    command_id = payload["command_id"]
    Process.send_after(self(), {:command_cancelled, command_id}, 20)
    {:noreply, state}
  end

  defp handle_command_kill(state, payload) do
    command_id = payload["command_id"]
    Process.send_after(self(), {:command_killed, command_id}, 10)
    {:noreply, state}
  end

  def handle_info({:session_updated, _session}, state), do: {:noreply, state}

  def handle_info({:command_started, command_id}, state) do
    state = emit(state, "command.started", %{"command_id" => command_id, "pid" => :erlang.phash2(command_id, 65_535), "started_at" => DateTime.to_iso8601(DateTime.utc_now())})
    state = emit(state, "command.stdout", %{"command_id" => command_id, "chunk" => "$ running\n", "encoding" => "utf-8", "stream_seq" => next_stream_seq(state, command_id)})
    Process.send_after(self(), {:command_exit, command_id}, 50)
    {:noreply, state}
  end

  def handle_info({:command_exit, command_id}, state) do
    if Map.has_key?(state.commands, command_id) do
      state = emit(state, "command.stdout", %{"command_id" => command_id, "chunk" => "done\n", "encoding" => "utf-8", "stream_seq" => next_stream_seq(state, command_id)})
      state = emit(state, "command.exit", %{"command_id" => command_id, "exit_code" => 0, "completed_at" => DateTime.to_iso8601(DateTime.utc_now())})
      {:noreply, %{state | commands: Map.delete(state.commands, command_id)}}
    else
      {:noreply, state}
    end
  end

  def handle_info({:command_cancelled, command_id}, state) do
    if Map.has_key?(state.commands, command_id) do
      state = emit(state, "command.cancelled", %{"command_id" => command_id, "completed_at" => DateTime.to_iso8601(DateTime.utc_now())})
      {:noreply, %{state | commands: Map.delete(state.commands, command_id)}}
    else
      {:noreply, state}
    end
  end

  def handle_info({:command_killed, command_id}, state) do
    if Map.has_key?(state.commands, command_id) do
      state = emit(state, "command.killed", %{"command_id" => command_id, "signal" => "SIGKILL", "completed_at" => DateTime.to_iso8601(DateTime.utc_now())})
      {:noreply, %{state | commands: Map.delete(state.commands, command_id)}}
    else
      {:noreply, state}
    end
  end

  def terminate(_reason, state) do
    SeedSessions.disconnect(state.session_id)
    :ok
  end

  defp maybe_ack_garden_message(state, %{"expects_ack" => true, "message_id" => message_id, "request_id" => request_id}) do
    emit(state, "ack", %{"status" => "accepted"}, %{
      ack_id: message_id,
      request_id: request_id,
      expects_ack: false,
      reply_to: message_id
    })
  end

  defp maybe_ack_garden_message(state, _message), do: state

  defp emit(state, type, payload, opts \\ %{}) do
    seq = state.seq + 1

    {:ok, message} =
      Message.validate(%{
        "version" => "1",
        "type" => type,
        "message_id" => Map.get(opts, :message_id, "sim_#{seq}_#{System.unique_integer([:positive])}"),
        "ack_id" => Map.get(opts, :ack_id),
        "request_id" => Map.get(opts, :request_id, "sim_req_#{System.unique_integer([:positive])}"),
        "session_id" => state.session_id,
        "sandbox_id" => state.sandbox_id,
        "seq" => seq,
        "timestamp" => DateTime.to_iso8601(DateTime.utc_now()),
        "expects_ack" => Map.get(opts, :expects_ack, true),
        "reply_to" => Map.get(opts, :reply_to),
        "payload" => payload
      })

    {:ok, _} = SeedSessions.record_inbound(state.session_id, message)
    %{state | seq: seq}
  end

  defp via(session_id), do: {:global, {__MODULE__, session_id}}

  defp garden_seq(state) do
    {:ok, session} = SeedSessions.get(state.session_id)
    session.last_garden_seq_sent
  end

  defp next_stream_seq(state, command_id) do
    state.commands
    |> Map.get(command_id, %{})
    |> Map.get("stream_seq", 0)
    |> Kernel.+(1)
  end
end
