defmodule Garden.Sandboxes.Store do
  @moduledoc false

  use GenServer

  alias Garden.Persistence
  alias Garden.SandboxBackend
  alias Garden.Sandboxes
  alias Garden.SeedSessions

  def start_link(arg \\ []) do
    GenServer.start_link(__MODULE__, arg, name: __MODULE__)
  end

  def reset!, do: GenServer.call(__MODULE__, :reset)
  def list_sandboxes, do: GenServer.call(__MODULE__, :list_sandboxes)
  def acquire_sandbox(attrs), do: GenServer.call(__MODULE__, {:acquire_sandbox, attrs})
  def ensure_sandbox(id, attrs), do: GenServer.call(__MODULE__, {:ensure_sandbox, id, attrs})
  def get_sandbox(id), do: GenServer.call(__MODULE__, {:get_sandbox, id})
  def extend_lease(id, attrs), do: GenServer.call(__MODULE__, {:extend_lease, id, attrs})
  def release_sandbox(id, attrs), do: GenServer.call(__MODULE__, {:release_sandbox, id, attrs})
  def list_sandbox_events(id, opts), do: GenServer.call(__MODULE__, {:list_sandbox_events, id, opts})
  def list_commands(sandbox_id), do: GenServer.call(__MODULE__, {:list_commands, sandbox_id})
  def start_command(sandbox_id, attrs), do: GenServer.call(__MODULE__, {:start_command, sandbox_id, attrs})
  def get_command(sandbox_id, command_id), do: GenServer.call(__MODULE__, {:get_command, sandbox_id, command_id})
  def send_stdin(sandbox_id, command_id, attrs), do: GenServer.call(__MODULE__, {:send_stdin, sandbox_id, command_id, attrs})
  def cancel_command(sandbox_id, command_id, attrs), do: GenServer.call(__MODULE__, {:cancel_command, sandbox_id, command_id, attrs})
  def kill_command(sandbox_id, command_id, attrs), do: GenServer.call(__MODULE__, {:kill_command, sandbox_id, command_id, attrs})
  def list_command_events(sandbox_id, command_id, opts), do: GenServer.call(__MODULE__, {:list_command_events, sandbox_id, command_id, opts})
  def list_files(sandbox_id, path), do: GenServer.call(__MODULE__, {:list_files, sandbox_id, path})
  def read_file(sandbox_id, path), do: GenServer.call(__MODULE__, {:read_file, sandbox_id, path})
  def write_file(sandbox_id, path, content), do: GenServer.call(__MODULE__, {:write_file, sandbox_id, path, content})
  def protocol_message(session_id, message), do: GenServer.cast(__MODULE__, {:protocol_message, session_id, message})

  @sweep_interval_ms 30_000
  @max_lease_ms 7_200_000
  @lease_warning_ms 300_000
  @lease_expiring_ms 60_000

  @impl true
  def init(_arg) do
    schedule_lease_sweep()
    {:ok,
     %{
       sandboxes: %{},
       commands: %{},
       filesystems: %{},
       sandbox_events: %{},
       command_events: %{},
       next_seq: %{},
       lease_warnings: %{}
     }}
  end

  @impl true
  def handle_call(:reset, _from, state) do
    Enum.each(Map.keys(state.sandboxes), &SandboxBackend.teardown_sandbox/1)

    {:reply, :ok,
     %{
       sandboxes: %{},
       commands: %{},
       filesystems: %{},
       sandbox_events: %{},
       command_events: %{},
       next_seq: %{},
       lease_warnings: %{}
     }}
  end

  def handle_call(:list_sandboxes, _from, state) do
    sandboxes =
      state.sandboxes
      |> Map.values()
      |> Enum.sort_by(& &1.inserted_at, {:desc, DateTime})

    {:reply, sandboxes, state}
  end

  def handle_call({:acquire_sandbox, attrs}, _from, state) do
    sandbox_id = id("sbx")
    now = now()
    ttl_ms = get_in(attrs, ["lease", "ttl_ms"]) || 30 * 60 * 1000

    sandbox = %{
      id: sandbox_id,
      environment: Map.get(attrs, "environment", "linux"),
      template: Map.get(attrs, "template", "default"),
      state: "provisioning",
      metadata: Map.put(Map.get(attrs, "metadata", %{}), "backend", SandboxBackend.name()),
      capabilities: %{"commands" => true, "files" => true, "pty" => false, "backend" => SandboxBackend.name()},
      lease: lease(ttl_ms, now),
      inserted_at: now,
      updated_at: now
    }

    state = put_sandbox(state, sandbox)
    state = maybe_seed_mock_filesystem(state, sandbox)
    state = append_sandbox_event(state, sandbox_id, "sandbox.provisioning", %{"state" => "provisioning", "backend" => SandboxBackend.name()})

    :ok = normalize_setup_result(SandboxBackend.setup_sandbox(sandbox, self()))

    response = %{
      sandbox: sandbox,
      operation: %{"id" => id("op"), "state" => "pending"}
    }

    {:reply, {:ok, response}, state}
  end

  def handle_call({:ensure_sandbox, id, attrs}, _from, state) do
    case fetch_sandbox(state, id) do
      {:ok, sandbox} ->
        {:reply, {:ok, sandbox}, state}

      {:error, :sandbox_not_found} ->
        now = now()

        sandbox = %{
          id: id,
          environment: Map.get(attrs, "environment", "linux"),
          template: Map.get(attrs, "template", "default"),
          state: Map.get(attrs, "state", "ready"),
          metadata: Map.put(Map.get(attrs, "metadata", %{"source" => "ui_seed_session"}), "backend", SandboxBackend.name()),
          capabilities: %{"commands" => true, "files" => true, "pty" => false, "backend" => SandboxBackend.name()},
          lease: lease(Map.get(attrs, "ttl_ms", 30 * 60 * 1000), now),
          inserted_at: now,
          updated_at: now
        }

        state = put_sandbox(state, sandbox)
        state = maybe_seed_mock_filesystem(state, sandbox)
        :ok = normalize_setup_result(SandboxBackend.setup_sandbox(sandbox, self()))
        state = append_sandbox_event(state, id, "sandbox.ready", %{"state" => "ready", "source" => "ensure_sandbox", "backend" => SandboxBackend.name()})
        {:reply, {:ok, sandbox}, state}
    end
  end

  def handle_call({:get_sandbox, id}, _from, state) do
    {:reply, fetch_sandbox(state, id), state}
  end

  def handle_call({:extend_lease, id, attrs}, _from, state) do
    with {:ok, sandbox} <- fetch_sandbox(state, id) do
      ttl_ms = Map.get(attrs, "ttl_ms", sandbox.lease["ttl_ms"])
      now = now()
      updated_lease = capped_lease(ttl_ms, now, sandbox.inserted_at)
      updated = %{sandbox | lease: updated_lease, updated_at: now}

      state = put_sandbox(state, updated)
      state = %{state | lease_warnings: Map.delete(state.lease_warnings, id)}

      state =
        append_sandbox_event(state, id, "sandbox.lease.extended", %{
          "ttl_ms" => ttl_ms,
          "reason" => Map.get(attrs, "reason", "unspecified"),
          "expires_at" => updated.lease["expires_at"]
        })

      {:reply, {:ok, updated.lease}, state}
    else
      error -> {:reply, error, state}
    end
  end

  def handle_call({:release_sandbox, id, attrs}, _from, state) do
    with {:ok, sandbox} <- fetch_sandbox(state, id) do
      releasing = %{sandbox | state: "releasing", updated_at: now()}
      state = put_sandbox(state, releasing)

      state =
        append_sandbox_event(state, id, "sandbox.release.requested", %{
          "reason" => Map.get(attrs, "reason", "unspecified")
        })

      SandboxBackend.teardown_sandbox(id)

      released = %{releasing | state: "released", updated_at: now()}
      state = put_sandbox(state, released)
      state = append_sandbox_event(state, id, "sandbox.released", %{"state" => "released"})

      {:reply, {:ok, released}, state}
    else
      error -> {:reply, error, state}
    end
  end

  def handle_call({:list_sandbox_events, id, opts}, _from, state) do
    if Map.has_key?(state.sandboxes, id) do
      after_seq = parse_after(opts)
      events = state.sandbox_events |> Map.get(id, []) |> Enum.filter(&(&1.seq > after_seq))
      next_cursor = case List.last(events) do nil -> Integer.to_string(after_seq); event -> Integer.to_string(event.seq) end
      {:reply, {:ok, %{events: events, next_cursor: next_cursor}}, state}
    else
      {:reply, {:error, :sandbox_not_found}, state}
    end
  end

  def handle_call({:list_commands, sandbox_id}, _from, state) do
    if Map.has_key?(state.sandboxes, sandbox_id) do
      commands = state.commands |> Map.get(sandbox_id, %{}) |> Map.values() |> Enum.sort_by(& &1.inserted_at, DateTime)
      {:reply, {:ok, commands}, state}
    else
      {:reply, {:error, :sandbox_not_found}, state}
    end
  end

  def handle_call({:start_command, sandbox_id, attrs}, _from, state) do
    with {:ok, sandbox} <- fetch_sandbox(state, sandbox_id),
         :ok <- ensure_ready(sandbox),
         {:ok, command_text} <- require_string(attrs, "command") do
      command = %{
        id: id("cmd"),
        sandbox_id: sandbox_id,
        state: "queued",
        command: command_text,
        cwd: Map.get(attrs, "cwd", "/workspace"),
        env: Map.get(attrs, "env", %{}),
        stdin: Map.get(attrs, "stdin", false),
        timeout_ms: Map.get(attrs, "timeout_ms", 60_000),
        metadata: Map.get(attrs, "metadata", %{}),
        pid: nil,
        exit_code: nil,
        signal: nil,
        started_at: nil,
        completed_at: nil,
        inserted_at: now(),
        updated_at: now()
      }

      state = put_command(state, command)
      state = append_command_event(state, sandbox_id, command.id, "command.queued", %{"command" => command.command})

      case dispatch_or_mock_start(sandbox_id, command) do
        :ok -> {:reply, {:ok, command}, state}
        {:error, reason} -> {:reply, {:error, reason}, state}
      end
    else
      {:error, _} = error -> {:reply, error, state}
    end
  end

  def handle_call({:get_command, sandbox_id, command_id}, _from, state) do
    {:reply, fetch_command(state, sandbox_id, command_id), state}
  end

  def handle_call({:send_stdin, sandbox_id, command_id, attrs}, _from, state) do
    with {:ok, command} <- fetch_command(state, sandbox_id, command_id),
         true <- command.stdin || {:error, :stdin_not_enabled},
         :ok <- dispatch_or_mock_stdin(sandbox_id, command_id, Map.get(attrs, "data", "")) do
      {:reply, {:ok, %{"accepted" => true}}, state}
    else
      false -> {:reply, {:error, :stdin_not_enabled}, state}
      {:error, _} = error -> {:reply, error, state}
    end
  end

  def handle_call({:cancel_command, sandbox_id, command_id, attrs}, _from, state) do
    with {:ok, command} <- fetch_command(state, sandbox_id, command_id),
         :ok <- ensure_running(command),
         :ok <- dispatch_or_mock_cancel(sandbox_id, command_id, attrs) do
      updated = %{command | state: "cancelling", updated_at: now()}
      state = put_command(state, updated)
      {:reply, {:ok, updated}, state}
    else
      {:error, _} = error -> {:reply, error, state}
    end
  end

  def handle_call({:kill_command, sandbox_id, command_id, _attrs}, _from, state) do
    with {:ok, command} <- fetch_command(state, sandbox_id, command_id),
         :ok <- ensure_running(command),
         :ok <- dispatch_or_mock_kill(sandbox_id, command_id) do
      updated = %{command | state: "killed", updated_at: now()}
      state = put_command(state, updated)
      {:reply, {:ok, updated}, state}
    else
      {:error, _} = error -> {:reply, error, state}
    end
  end

  def handle_call({:list_files, sandbox_id, path}, _from, state) do
    with {:ok, sandbox} <- fetch_sandbox(state, sandbox_id),
         {:ok, %{entries: entries}} <- SandboxBackend.list_files(sandbox, path) do
      {:reply, {:ok, %{path: path, entries: entries}}, state}
    else
      error -> {:reply, error, state}
    end
  end

  def handle_call({:read_file, sandbox_id, path}, _from, state) do
    with {:ok, sandbox} <- fetch_sandbox(state, sandbox_id),
         {:ok, result} <- SandboxBackend.read_file(sandbox, path) do
      {:reply, {:ok, result}, state}
    else
      error -> {:reply, error, state}
    end
  end

  def handle_call({:write_file, sandbox_id, path, content}, _from, state) do
    with {:ok, sandbox} <- fetch_sandbox(state, sandbox_id),
         {:ok, result} <- SandboxBackend.write_file(sandbox, path, content) do
      state = append_sandbox_event(state, sandbox_id, "file.written", %{"path" => path, "bytes" => byte_size(content)})
      {:reply, {:ok, result}, state}
    else
      error -> {:reply, error, state}
    end
  end

  def handle_call({:list_command_events, sandbox_id, command_id, opts}, _from, state) do
    with {:ok, _command} <- fetch_command(state, sandbox_id, command_id) do
      after_seq = parse_after(opts)

      events =
        state.command_events
        |> Map.get({sandbox_id, command_id}, [])
        |> Enum.filter(&(&1.seq > after_seq))

      next_cursor = case List.last(events) do nil -> Integer.to_string(after_seq); event -> Integer.to_string(event.seq) end
      {:reply, {:ok, %{events: events, next_cursor: next_cursor}}, state}
    else
      error -> {:reply, error, state}
    end
  end

  @impl true
  def handle_cast({:sandbox_transition, sandbox_id, state_name}, state) do
    case fetch_sandbox(state, sandbox_id) do
      {:ok, sandbox} ->
        updated = %{sandbox | state: Atom.to_string(state_name), updated_at: now()}
        state = put_sandbox(state, updated)
        state = append_sandbox_event(state, sandbox_id, "sandbox.#{state_name}", %{"state" => Atom.to_string(state_name)})
        {:noreply, state}

      {:error, :sandbox_not_found} ->
        {:noreply, state}
    end
  end

  def handle_cast({:command_started, sandbox_id, command_id, pid}, state) do
    case fetch_command(state, sandbox_id, command_id) do
      {:ok, command} ->
        updated = %{
          command
          | state: "running",
            pid: pid,
            started_at: now(),
            updated_at: now()
        }

        state = put_command(state, updated)
        state = append_command_event(state, sandbox_id, command_id, "command.started", %{"pid" => pid})
        state = touch_lease(state, sandbox_id, "command_started")
        {:noreply, state}

      {:error, :command_not_found} ->
        {:noreply, state}
    end
  end

  def handle_cast({:command_output, sandbox_id, command_id, stream, chunk}, state) do
    state = append_command_event(state, sandbox_id, command_id, "command.#{stream}", %{"chunk" => chunk})
    state = touch_lease(state, sandbox_id, "command_output")
    {:noreply, state}
  end

  def handle_cast({:command_stdin, sandbox_id, command_id, data}, state) do
    state = append_command_event(state, sandbox_id, command_id, "command.stdin.accepted", %{"data" => data})
    state = touch_lease(state, sandbox_id, "stdin")
    {:noreply, state}
  end

  def handle_cast({:command_cancelling, sandbox_id, command_id, grace_period_ms}, state) do
    state = append_command_event(state, sandbox_id, command_id, "command.cancel.requested", %{"grace_period_ms" => grace_period_ms})
    {:noreply, state}
  end

  def handle_cast({:command_kill_requested, sandbox_id, command_id}, state) do
    state = append_command_event(state, sandbox_id, command_id, "command.kill.requested", %{})
    {:noreply, state}
  end

  def handle_cast({:protocol_message, _session_id, %{"sandbox_id" => sandbox_id, "type" => type, "payload" => payload}}, state) do
    state =
      case type do
        "seed.register" ->
          update_sandbox_from_protocol(state, sandbox_id, "ready")

        "seed.status" ->
          update_sandbox_from_protocol(state, sandbox_id, payload["state"])

        "command.accepted" ->
          update_command_from_protocol(state, sandbox_id, payload["command_id"], fn command ->
            %{command | state: payload["state"] || "starting", updated_at: now()}
          end, "command.accepted", payload)

        "command.started" ->
          update_command_from_protocol(state, sandbox_id, payload["command_id"], fn command ->
            %{command | state: "running", pid: payload["pid"], started_at: now(), updated_at: now()}
          end, "command.started", payload)

        "command.stdout" ->
          append_command_event(state, sandbox_id, payload["command_id"], "command.stdout", %{"chunk" => payload["chunk"]})

        "command.stderr" ->
          append_command_event(state, sandbox_id, payload["command_id"], "command.stderr", %{"chunk" => payload["chunk"]})

        "command.stdin.accepted" ->
          append_command_event(state, sandbox_id, payload["command_id"], "command.stdin.accepted", payload)

        "command.exit" ->
          finish_protocol_command(state, sandbox_id, payload["command_id"], :exited, %{exit_code: payload["exit_code"]})

        "command.failed" ->
          finish_protocol_command(state, sandbox_id, payload["command_id"], :failed, %{exit_code: payload["exit_code"]})

        "command.cancelled" ->
          finish_protocol_command(state, sandbox_id, payload["command_id"], :killed, %{})

        "command.killed" ->
          finish_protocol_command(state, sandbox_id, payload["command_id"], :killed, %{signal: payload["signal"]})

        _ ->
          state
      end

    {:noreply, state}
  end

  def handle_cast({:command_finished, sandbox_id, command_id, status, attrs}, state) do
    case fetch_command(state, sandbox_id, command_id) do
      {:ok, command} when command.state in ["exited", "failed", "killed"] ->
        {:noreply, state}

      {:ok, command} ->
        updated =
          command
          |> Map.put(:state, Atom.to_string(status))
          |> Map.put(:completed_at, now())
          |> Map.put(:updated_at, now())
          |> Map.put(:exit_code, Map.get(attrs, :exit_code))
          |> Map.put(:signal, Map.get(attrs, :signal))

        state = put_command(state, updated)

        event_type =
          case status do
            :exited -> "command.exit"
            :failed -> "command.failed"
            :killed -> "command.killed"
          end

        payload = %{}
        |> maybe_put("exit_code", updated.exit_code)
        |> maybe_put("signal", updated.signal)

        state = append_command_event(state, sandbox_id, command_id, event_type, payload)
        {:noreply, state}

      {:error, :command_not_found} ->
        {:noreply, state}
    end
  end

  @impl true
  def handle_info(:sweep_leases, state) do
    now = now()

    state =
      state.sandboxes
      |> Map.values()
      |> Enum.filter(&(&1.state not in ["releasing", "released", "failed"]))
      |> Enum.reduce(state, fn sandbox, acc ->
        id = sandbox.id
        remaining_ms = remaining_lease_ms(sandbox, now)
        warnings = Map.get(acc.lease_warnings, id, MapSet.new())

        cond do
          remaining_ms <= 0 ->
            releasing = %{sandbox | state: "releasing", updated_at: now}
            acc = put_sandbox(acc, releasing)
            acc = append_sandbox_event(acc, id, "sandbox.release.requested", %{"reason" => "lease_expired"})
            SandboxBackend.teardown_sandbox(id)
            released = %{releasing | state: "released", updated_at: now}
            acc = put_sandbox(acc, released)
            append_sandbox_event(acc, id, "sandbox.released", %{"state" => "released"})

          remaining_ms <= @lease_expiring_ms and not MapSet.member?(warnings, :expiring) ->
            dispatch_lease_warning(id, "garden.lease_expiring", remaining_ms)
            %{acc | lease_warnings: Map.put(acc.lease_warnings, id, MapSet.put(warnings, :expiring))}

          remaining_ms <= @lease_warning_ms and not MapSet.member?(warnings, :warning) ->
            dispatch_lease_warning(id, "garden.lease_warning", remaining_ms)
            %{acc | lease_warnings: Map.put(acc.lease_warnings, id, MapSet.put(warnings, :warning))}

          true ->
            acc
        end
      end)

    schedule_lease_sweep()
    {:noreply, state}
  end

  defp dispatch_or_mock_start(sandbox_id, command) do
    case SeedSessions.find_by_sandbox(sandbox_id) do
      {:ok, %{status: :connected} = session} ->
        SeedSessions.dispatch(session.session_id, "command.start", %{
          "command_id" => command.id,
          "command" => command.command,
          "cwd" => command.cwd,
          "env" => command.env,
          "stdin" => command.stdin,
          "timeout_ms" => command.timeout_ms,
          "metadata" => command.metadata
        })
        :ok

      _ ->
        SandboxBackend.start_command(sandbox_id, command)
    end
  end

  defp dispatch_or_mock_stdin(sandbox_id, command_id, data) do
    case SeedSessions.find_by_sandbox(sandbox_id) do
      {:ok, %{status: :connected} = session} ->
        SeedSessions.dispatch(session.session_id, "command.stdin", %{"command_id" => command_id, "data" => data, "encoding" => "utf-8"})
        :ok

      _ ->
        SandboxBackend.send_stdin(sandbox_id, command_id, data)
    end
  end

  defp dispatch_or_mock_cancel(sandbox_id, command_id, attrs) do
    case SeedSessions.find_by_sandbox(sandbox_id) do
      {:ok, %{status: :connected} = session} ->
        SeedSessions.dispatch(session.session_id, "command.cancel", %{"command_id" => command_id, "grace_period_ms" => Map.get(attrs, "grace_period_ms", 5_000), "escalation" => "kill"})
        :ok

      _ ->
        SandboxBackend.cancel_command(sandbox_id, command_id, attrs)
    end
  end

  defp dispatch_or_mock_kill(sandbox_id, command_id) do
    case SeedSessions.find_by_sandbox(sandbox_id) do
      {:ok, %{status: :connected} = session} ->
        SeedSessions.dispatch(session.session_id, "command.kill", %{"command_id" => command_id})
        :ok

      _ ->
        SandboxBackend.kill_command(sandbox_id, command_id)
    end
  end

  defp update_sandbox_from_protocol(state, sandbox_id, sandbox_state) when is_binary(sandbox_state) do
    case fetch_sandbox(state, sandbox_id) do
      {:ok, sandbox} ->
        updated = %{sandbox | state: sandbox_state, updated_at: now()}
        state = put_sandbox(state, updated)
        append_sandbox_event(state, sandbox_id, "sandbox.#{sandbox_state}", %{"state" => sandbox_state})
      _ ->
        state
    end
  end

  defp update_sandbox_from_protocol(state, _sandbox_id, _sandbox_state), do: state

  defp update_command_from_protocol(state, sandbox_id, command_id, fun, event_type, payload) do
    case fetch_command(state, sandbox_id, command_id) do
      {:ok, command} ->
        state
        |> put_command(fun.(command))
        |> append_command_event(sandbox_id, command_id, event_type, payload)
        |> touch_lease(sandbox_id, event_type)

      _ ->
        state
    end
  end

  defp finish_protocol_command(state, sandbox_id, command_id, status, attrs) do
    case fetch_command(state, sandbox_id, command_id) do
      {:ok, command} ->
        updated =
          command
          |> Map.put(:state, Atom.to_string(status))
          |> Map.put(:completed_at, now())
          |> Map.put(:updated_at, now())
          |> Map.put(:exit_code, Map.get(attrs, :exit_code))
          |> Map.put(:signal, Map.get(attrs, :signal))

        event_type =
          case status do
            :exited -> "command.exit"
            :failed -> "command.failed"
            :killed -> "command.killed"
          end

        payload = %{}
        |> maybe_put("exit_code", updated.exit_code)
        |> maybe_put("signal", updated.signal)

        state
        |> put_command(updated)
        |> append_command_event(sandbox_id, command_id, event_type, payload)
        |> touch_lease(sandbox_id, event_type)

      _ ->
        state
    end
  end

  defp maybe_seed_mock_filesystem(state, sandbox) do
    case SandboxBackend.name() do
      "mock" -> put_filesystem(state, sandbox.id, default_filesystem(sandbox.id))
      _ -> state
    end
  end

  defp normalize_setup_result(:ok), do: :ok
  defp normalize_setup_result({:ok, _pid}), do: :ok
  defp normalize_setup_result(other), do: other

  defp put_filesystem(state, sandbox_id, filesystem) do
    put_in(state, [:filesystems, sandbox_id], filesystem)
  end

  defp default_filesystem(sandbox_id) do
    %{
      "/workspace/README.txt" => "Sandbox #{sandbox_id}\n",
      "/workspace/notes.txt" => "hello from garden\n"
    }
  end

  defp fetch_sandbox(state, id) do
    case Map.fetch(state.sandboxes, id) do
      {:ok, sandbox} -> {:ok, sandbox}
      :error -> {:error, :sandbox_not_found}
    end
  end

  defp fetch_command(state, sandbox_id, command_id) do
    case state.commands |> Map.get(sandbox_id, %{}) |> Map.fetch(command_id) do
      {:ok, command} -> {:ok, command}
      :error -> {:error, :command_not_found}
    end
  end

  defp ensure_ready(%{state: "ready"}), do: :ok
  defp ensure_ready(_sandbox), do: {:error, :sandbox_not_ready}

  defp ensure_running(%{state: state}) when state in ["queued", "starting", "running", "cancelling"], do: :ok
  defp ensure_running(_command), do: {:error, :command_not_running}

  defp put_sandbox(state, sandbox) do
    Persistence.persist_sandbox(sandbox)
    put_in(state, [:sandboxes, sandbox.id], sandbox)
  end

  defp put_command(state, command) do
    Persistence.persist_command(command)

    update_in(state, [:commands], fn commands ->
      Map.update(commands, command.sandbox_id, %{command.id => command}, &Map.put(&1, command.id, command))
    end)
  end

  defp append_sandbox_event(state, sandbox_id, type, data) do
    {state, event} = next_event(state, sandbox_id, type, data, nil)
    Persistence.persist_sandbox_event(event)
    broadcast(Sandboxes.sandbox_topic(sandbox_id), event)
    update_in(state, [:sandbox_events, sandbox_id], fn events -> (events || []) ++ [event] end)
  end

  defp append_command_event(state, sandbox_id, command_id, type, data) do
    {state, event} = next_event(state, sandbox_id, type, data, command_id)
    Persistence.persist_command_event(event)
    broadcast(Sandboxes.command_topic(sandbox_id, command_id), event)
    update_in(state, [:command_events, {sandbox_id, command_id}], fn events -> (events || []) ++ [event] end)
  end

  defp next_event(state, sandbox_id, type, data, command_id) do
    seq = Map.get(state.next_seq, sandbox_id, 0) + 1
    event = %{
      id: id("evt"),
      seq: seq,
      cursor: Integer.to_string(seq),
      type: type,
      sandbox_id: sandbox_id,
      command_id: command_id,
      timestamp: DateTime.to_iso8601(now()),
      data: data
    }

    {put_in(state, [:next_seq, sandbox_id], seq), event}
  end

  defp require_string(attrs, key) do
    case Map.get(attrs, key) do
      value when is_binary(value) and value != "" -> {:ok, value}
      _ -> {:error, :invalid_request}
    end
  end

  defp parse_after(opts) do
    opts
    |> Map.get("after", Map.get(opts, :after, 0))
    |> case do
      value when is_integer(value) -> value
      value when is_binary(value) -> String.to_integer(value)
      _ -> 0
    end
  rescue
    _ -> 0
  end

  defp touch_lease(state, sandbox_id, reason) do
    case fetch_sandbox(state, sandbox_id) do
      {:ok, sandbox} ->
        now = now()
        ttl_ms = sandbox.lease["ttl_ms"]
        updated = %{sandbox | lease: capped_lease(ttl_ms, now, sandbox.inserted_at), updated_at: now}
        state = put_sandbox(state, updated)
        state = %{state | lease_warnings: Map.delete(state.lease_warnings, sandbox_id)}
        append_sandbox_event(state, sandbox_id, "sandbox.lease.extended", %{"ttl_ms" => ttl_ms, "reason" => reason, "expires_at" => updated.lease["expires_at"]})

      _ ->
        state
    end
  end

  defp capped_lease(ttl_ms, from, inserted_at) do
    max_expires = DateTime.add(inserted_at, @max_lease_ms, :millisecond)
    raw_expires = DateTime.add(from, ttl_ms, :millisecond)
    expires = if DateTime.compare(raw_expires, max_expires) == :gt, do: max_expires, else: raw_expires
    %{"ttl_ms" => ttl_ms, "expires_at" => DateTime.to_iso8601(expires)}
  end

  defp lease(ttl_ms, from) do
    %{
      "ttl_ms" => ttl_ms,
      "expires_at" => from |> DateTime.add(ttl_ms, :millisecond) |> DateTime.to_iso8601()
    }
  end

  defp maybe_put(map, _key, nil), do: map
  defp maybe_put(map, key, value), do: Map.put(map, key, value)

  defp broadcast(topic, event) do
    Phoenix.PubSub.broadcast(Garden.PubSub, topic, {:event, event})
  end

  defp schedule_lease_sweep, do: Process.send_after(self(), :sweep_leases, @sweep_interval_ms)

  defp remaining_lease_ms(sandbox, now) do
    case DateTime.from_iso8601(sandbox.lease["expires_at"]) do
      {:ok, expires_at, _} -> DateTime.diff(expires_at, now, :millisecond)
      _ -> 0
    end
  end

  defp dispatch_lease_warning(sandbox_id, type, remaining_ms) do
    case SeedSessions.find_by_sandbox(sandbox_id) do
      {:ok, %{status: :connected} = session} ->
        SeedSessions.dispatch(session.session_id, type, %{
          "sandbox_id" => sandbox_id,
          "remaining_ms" => remaining_ms
        })
      _ ->
        :ok
    end
  end

  defp id(prefix), do: prefix <> "_" <> Base.encode16(:crypto.strong_rand_bytes(6), case: :lower)
  defp now, do: DateTime.utc_now() |> DateTime.truncate(:second)
end
