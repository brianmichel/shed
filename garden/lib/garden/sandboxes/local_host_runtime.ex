defmodule Garden.Sandboxes.LocalHostRuntime do
  use GenServer

  alias Garden.Guardrails

  def start_link(opts) do
    sandbox_id = Keyword.fetch!(opts, :sandbox_id)
    GenServer.start_link(__MODULE__, opts, name: via(sandbox_id))
  end

  def ensure_started(sandbox_id, store, root, sandbox) do
    case :global.whereis_name({__MODULE__, sandbox_id}) do
      :undefined -> Garden.Sandboxes.LocalHostRuntimeSupervisor.start_runtime(sandbox_id, store, root, sandbox)
      _pid -> GenServer.cast(via(sandbox_id), {:update_store, store})
    end
  end

  def stop(sandbox_id) do
    GenServer.stop(via(sandbox_id), :normal)
  catch
    :exit, _ -> :ok
  end

  def start_command(sandbox_id, command), do: GenServer.call(via(sandbox_id), {:start_command, command})
  def send_stdin(sandbox_id, command_id, data), do: GenServer.call(via(sandbox_id), {:send_stdin, command_id, data})
  def cancel_command(sandbox_id, command_id, attrs), do: GenServer.call(via(sandbox_id), {:cancel_command, command_id, attrs})
  def kill_command(sandbox_id, command_id), do: GenServer.call(via(sandbox_id), {:kill_command, command_id})

  def init(opts) do
    state = %{sandbox_id: opts[:sandbox_id], store: opts[:store], root: opts[:root], sandbox: opts[:sandbox], commands: %{}}
    Process.send_after(self(), :booting, 10)
    Process.send_after(self(), :ready, 25)
    {:ok, state}
  end

  def handle_info(:booting, state), do: notify(state, {:sandbox_transition, state.sandbox_id, :booting}) |> noreply(state)
  def handle_info(:ready, state), do: notify(state, {:sandbox_transition, state.sandbox_id, :ready}) |> noreply(state)

  def handle_info({port, {:data, data}}, state) do
    case find_command_by_port(state, port) do
      {command_id, %{stream: stream}} ->
        notify(state, {:command_output, state.sandbox_id, command_id, stream, IO.iodata_to_binary(data)})
        {:noreply, state}
      _ -> {:noreply, state}
    end
  end

  def handle_info({port, {:exit_status, status}}, state) do
    case find_command_by_port(state, port) do
      {command_id, _info} ->
        kind = if status == 0, do: :exited, else: :failed
        attrs = if status == 0, do: %{exit_code: status}, else: %{exit_code: status}
        notify(state, {:command_finished, state.sandbox_id, command_id, kind, attrs})
        {:noreply, %{state | commands: Map.delete(state.commands, command_id)}}
      _ -> {:noreply, state}
    end
  end

  def handle_call({:start_command, command}, _from, state) do
    resolved_cwd = resolve_virtual_cwd(state, command.cwd)
    sandbox = Map.put(state.sandbox, :workspace_root, state.root)
    spec = %{command: command.command, cwd: resolved_cwd, env: command.env, metadata: command.metadata}

    with :ok <- Guardrails.allow_command(sandbox, spec),
         {:ok, cwd} <- Guardrails.normalize_cwd(sandbox, resolved_cwd),
         env <- Guardrails.sanitize_env(sandbox, command.env || %{}) do
      port = Port.open({:spawn, command.command}, [:binary, :stderr_to_stdout, :exit_status, {:cd, cwd}, {:env, Enum.into(env, [])}])
      os_pid = case Port.info(port, :os_pid) do {:os_pid, pid} -> pid; _ -> nil end
      notify(state, {:command_started, state.sandbox_id, command.id, os_pid || :erlang.phash2(command.id, 65_535)})
      {:reply, :ok, %{state | commands: Map.put(state.commands, command.id, %{port: port, os_pid: os_pid, stream: :stdout})}}
    else
      {:error, reason} -> {:reply, {:error, reason}, state}
    end
  end

  def handle_call({:send_stdin, command_id, data}, _from, state) do
    case Map.get(state.commands, command_id) do
      %{port: port} ->
        Port.command(port, data)
        notify(state, {:command_stdin, state.sandbox_id, command_id, data})
        {:reply, :ok, state}
      _ -> {:reply, {:error, :command_not_found}, state}
    end
  end

  def handle_call({:cancel_command, command_id, attrs}, _from, state) do
    with %{os_pid: os_pid} <- Map.get(state.commands, command_id),
         :ok <- Guardrails.allow_signal(Map.put(state.sandbox, :workspace_root, state.root), "SIGTERM") do
      notify(state, {:command_cancelling, state.sandbox_id, command_id, Map.get(attrs, "grace_period_ms", 5000)})
      System.cmd("kill", ["-TERM", Integer.to_string(os_pid)])
      {:reply, :ok, state}
    else
      nil -> {:reply, {:error, :command_not_found}, state}
      {:error, reason} -> {:reply, {:error, reason}, state}
    end
  end

  def handle_call({:kill_command, command_id}, _from, state) do
    with %{os_pid: os_pid} <- Map.get(state.commands, command_id),
         :ok <- Guardrails.allow_signal(Map.put(state.sandbox, :workspace_root, state.root), "SIGKILL") do
      notify(state, {:command_kill_requested, state.sandbox_id, command_id})
      System.cmd("kill", ["-KILL", Integer.to_string(os_pid)])
      {:reply, :ok, state}
    else
      nil -> {:reply, {:error, :command_not_found}, state}
      {:error, reason} -> {:reply, {:error, reason}, state}
    end
  end

  def handle_cast({:update_store, store}, state) do
    {:noreply, %{state | store: store}}
  end

  defp find_command_by_port(state, port) do
    Enum.find_value(state.commands, fn {id, info} -> if info.port == port, do: {id, info} end)
  end

  defp notify(state, msg) do
    GenServer.cast(state.store, msg)
  end

  defp noreply(_msg, state), do: {:noreply, state}
  defp via(sandbox_id), do: {:global, {__MODULE__, sandbox_id}}

  defp resolve_virtual_cwd(state, nil), do: state.root
  defp resolve_virtual_cwd(state, "/workspace"), do: state.root
  defp resolve_virtual_cwd(state, "/workspace/" <> rest), do: Path.join(state.root, rest)
  defp resolve_virtual_cwd(state, cwd), do: Path.expand(cwd, state.root)
end
