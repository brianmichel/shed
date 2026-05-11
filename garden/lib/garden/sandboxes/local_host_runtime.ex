defmodule Garden.Sandboxes.LocalHostRuntime do
  use GenServer

  require Logger

  def start_link(opts) do
    sandbox_id = Keyword.fetch!(opts, :sandbox_id)
    GenServer.start_link(__MODULE__, opts, name: via(sandbox_id))
  end

  def ensure_started(sandbox_id, store, root, sandbox, session) do
    case :global.whereis_name({__MODULE__, sandbox_id}) do
      :undefined -> Garden.Sandboxes.LocalHostRuntimeSupervisor.start_runtime(sandbox_id, store, root, sandbox, session)
      _pid -> :ok
    end
  end

  def stop(sandbox_id) do
    GenServer.stop(via(sandbox_id), :normal)
  catch
    :exit, _ -> :ok
  end

  # Only reached if no Seed session is connected yet.
  def start_command(sandbox_id, _command), do: call(sandbox_id, :no_session)
  def send_stdin(sandbox_id, _command_id, _data), do: call(sandbox_id, :no_session)
  def cancel_command(sandbox_id, _command_id, _attrs), do: call(sandbox_id, :no_session)
  def kill_command(sandbox_id, _command_id), do: call(sandbox_id, :no_session)

  @impl true
  def init(opts) do
    state = %{
      sandbox_id: opts[:sandbox_id],
      store: opts[:store],
      root: opts[:root],
      sandbox: opts[:sandbox],
      session: opts[:session],
      port: nil
    }

    send(self(), :start_daemon)
    {:ok, state}
  end

  @impl true
  def handle_info(:start_daemon, state) do
    case find_seed_binary() do
      {:ok, binary} ->
        session = state.session
        port =
          try do
            Port.open({:spawn_executable, binary}, [
              :binary,
              :exit_status,
              :stderr_to_stdout,
              {:args,
               [
                 "--url", seed_websocket_url(),
                 "--session-key", session.session_key,
                 "--sandbox-id", state.sandbox_id,
                 "--session-id", session.session_id,
                 "--workspace-root", state.root
               ]}
            ])
          rescue
            e ->
              Logger.error("[LocalHostRuntime] Port.open failed for #{state.sandbox_id}: #{inspect(e)}")
              nil
          end

        if port do
          notify(state, {:sandbox_transition, state.sandbox_id, :booting})
          {:noreply, %{state | port: port}}
        else
          notify(state, {:sandbox_transition, state.sandbox_id, :failed})
          {:stop, :port_open_failed, state}
        end

      {:error, reason} ->
        Logger.error("[LocalHostRuntime] seed binary not found for #{state.sandbox_id}: #{inspect(reason)}")
        notify(state, {:sandbox_transition, state.sandbox_id, :failed})
        {:stop, {:daemon_start_failed, reason}, state}
    end
  end

  def handle_info({port, {:data, data}}, %{port: port} = state) do
    Logger.debug("[seed/#{state.sandbox_id}] #{String.trim(data)}")
    {:noreply, state}
  end

  def handle_info({port, {:exit_status, code}}, %{port: port} = state) do
    Logger.warning("[LocalHostRuntime] seed daemon for #{state.sandbox_id} exited with code #{code}")
    notify(state, {:sandbox_transition, state.sandbox_id, :failed})
    {:stop, {:daemon_exited, code}, state}
  end

  def handle_info(_msg, state), do: {:noreply, state}

  @impl true
  def handle_call(:no_session, _from, state) do
    {:reply, {:error, :seed_not_connected}, state}
  end

  @impl true
  def terminate(_reason, state) do
    if state.port, do: Port.close(state.port)
    :ok
  end

  defp find_seed_binary do
    path =
      Application.get_env(:garden, :seed_binary) ||
        Path.expand("../../../seed/seed", __DIR__)

    if File.exists?(path) do
      {:ok, path}
    else
      {:error, {:seed_binary_not_found, path}}
    end
  end

  defp seed_websocket_url do
    Application.get_env(:garden, :seed_websocket_url, "ws://localhost:4000/ws/seed")
  end

  defp notify(state, msg), do: GenServer.cast(state.store, msg)
  defp via(sandbox_id), do: {:global, {__MODULE__, sandbox_id}}
  defp call(sandbox_id, msg), do: GenServer.call(via(sandbox_id), msg)
end
