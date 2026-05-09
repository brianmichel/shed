defmodule Garden.Sandboxes.Store do
  @moduledoc false

  use GenServer

  alias Garden.Sandboxes
  alias Garden.Sandboxes.MockCompute
  alias Garden.Sandboxes.MockComputeSupervisor

  def start_link(arg \\ []) do
    GenServer.start_link(__MODULE__, arg, name: __MODULE__)
  end

  def reset!, do: GenServer.call(__MODULE__, :reset)
  def list_sandboxes, do: GenServer.call(__MODULE__, :list_sandboxes)
  def acquire_sandbox(attrs), do: GenServer.call(__MODULE__, {:acquire_sandbox, attrs})
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

  @impl true
  def init(_arg) do
    {:ok,
     %{
       sandboxes: %{},
       commands: %{},
       sandbox_events: %{},
       command_events: %{},
       next_seq: %{}
     }}
  end

  @impl true
  def handle_call(:reset, _from, state) do
    Enum.each(Map.keys(state.sandboxes), &MockCompute.terminate_compute/1)

    {:reply, :ok,
     %{
       sandboxes: %{},
       commands: %{},
       sandbox_events: %{},
       command_events: %{},
       next_seq: %{}
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
      metadata: Map.get(attrs, "metadata", %{}),
      capabilities: %{"commands" => true, "files" => false, "pty" => false},
      lease: lease(ttl_ms, now),
      inserted_at: now,
      updated_at: now
    }

    state = put_sandbox(state, sandbox)
    state = append_sandbox_event(state, sandbox_id, "sandbox.provisioning", %{"state" => "provisioning"})

    {:ok, _pid} = MockComputeSupervisor.start_compute(sandbox_id, self())

    response = %{
      sandbox: sandbox,
      operation: %{"id" => id("op"), "state" => "pending"}
    }

    {:reply, {:ok, response}, state}
  end

  def handle_call({:get_sandbox, id}, _from, state) do
    {:reply, fetch_sandbox(state, id), state}
  end

  def handle_call({:extend_lease, id, attrs}, _from, state) do
    with {:ok, sandbox} <- fetch_sandbox(state, id) do
      ttl_ms = Map.get(attrs, "ttl_ms", sandbox.lease["ttl_ms"])
      updated = %{sandbox | lease: lease(ttl_ms, now()), updated_at: now()}

      state = put_sandbox(state, updated)

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

      MockCompute.terminate_compute(id)

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
      :ok = MockCompute.start_command(sandbox_id, command)
      {:reply, {:ok, command}, state}
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
         :ok <- MockCompute.send_stdin(sandbox_id, command_id, Map.get(attrs, "data", "")) do
      {:reply, {:ok, %{"accepted" => true}}, state}
    else
      false -> {:reply, {:error, :stdin_not_enabled}, state}
      {:error, _} = error -> {:reply, error, state}
    end
  end

  def handle_call({:cancel_command, sandbox_id, command_id, attrs}, _from, state) do
    with {:ok, command} <- fetch_command(state, sandbox_id, command_id),
         :ok <- ensure_running(command),
         :ok <- MockCompute.cancel_command(sandbox_id, command_id, attrs) do
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
         :ok <- MockCompute.kill_command(sandbox_id, command_id) do
      updated = %{command | state: "killed", updated_at: now()}
      state = put_command(state, updated)
      {:reply, {:ok, updated}, state}
    else
      {:error, _} = error -> {:reply, error, state}
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
    put_in(state, [:sandboxes, sandbox.id], sandbox)
  end

  defp put_command(state, command) do
    update_in(state, [:commands], fn commands ->
      Map.update(commands, command.sandbox_id, %{command.id => command}, &Map.put(&1, command.id, command))
    end)
  end

  defp append_sandbox_event(state, sandbox_id, type, data) do
    {state, event} = next_event(state, sandbox_id, type, data, nil)
    broadcast(Sandboxes.sandbox_topic(sandbox_id), event)
    update_in(state, [:sandbox_events, sandbox_id], fn events -> (events || []) ++ [event] end)
  end

  defp append_command_event(state, sandbox_id, command_id, type, data) do
    {state, event} = next_event(state, sandbox_id, type, data, command_id)
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
        ttl_ms = sandbox.lease["ttl_ms"]
        updated = %{sandbox | lease: lease(ttl_ms, now()), updated_at: now()}
        state = put_sandbox(state, updated)
        append_sandbox_event(state, sandbox_id, "sandbox.lease.extended", %{"ttl_ms" => ttl_ms, "reason" => reason, "expires_at" => updated.lease["expires_at"]})

      _ ->
        state
    end
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

  defp id(prefix), do: prefix <> "_" <> Base.encode16(:crypto.strong_rand_bytes(6), case: :lower)
  defp now, do: DateTime.utc_now() |> DateTime.truncate(:second)
end
