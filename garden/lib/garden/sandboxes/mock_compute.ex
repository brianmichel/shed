defmodule Garden.Sandboxes.MockCompute do
  @moduledoc """
  Mock compute runtime for a sandbox.

  It simulates command lifecycle, output streaming, stdin handling, and
  cancellation so the Garden API can be developed and tested end-to-end before
  wiring in real containers or VMs.
  """

  use GenServer

  defstruct [:sandbox_id, :store, commands: %{}]

  def start_link(opts) do
    sandbox_id = Keyword.fetch!(opts, :sandbox_id)
    GenServer.start_link(__MODULE__, opts, name: via(sandbox_id))
  end

  def start_command(sandbox_id, command) do
    GenServer.call(via(sandbox_id), {:start_command, command})
  end

  def send_stdin(sandbox_id, command_id, data) do
    GenServer.call(via(sandbox_id), {:send_stdin, command_id, data})
  end

  def cancel_command(sandbox_id, command_id, opts) do
    GenServer.call(via(sandbox_id), {:cancel_command, command_id, opts})
  end

  def kill_command(sandbox_id, command_id) do
    GenServer.call(via(sandbox_id), {:kill_command, command_id})
  end

  def terminate_compute(sandbox_id) do
    GenServer.stop(via(sandbox_id), :normal)
  catch
    :exit, _ -> :ok
  end

  @impl true
  def init(opts) do
    sandbox_id = Keyword.fetch!(opts, :sandbox_id)
    store = Keyword.fetch!(opts, :store)

    Process.send_after(self(), :booting, 10)
    Process.send_after(self(), :ready, 25)

    {:ok, %__MODULE__{sandbox_id: sandbox_id, store: store}}
  end

  @impl true
  def handle_info(:booting, state) do
    notify(state, {:sandbox_transition, state.sandbox_id, :booting})
    {:noreply, state}
  end

  def handle_info(:ready, state) do
    notify(state, {:sandbox_transition, state.sandbox_id, :ready})
    {:noreply, state}
  end

  def handle_info({:emit_output, command_id, stream, chunk}, state) do
    notify(state, {:command_output, state.sandbox_id, command_id, stream, chunk})
    {:noreply, state}
  end

  def handle_info({:run_command, command}, state) do
    Process.send_after(self(), {:emit_output, command.id, :stdout, "$ #{command.command}\n"}, 0)

    case command_output(state, command) do
      {:ok, output, completion_ms} ->
        if output != "" do
          Process.send_after(self(), {:emit_output, command.id, :stdout, output}, 10)
        end

        Process.send_after(self(), {:complete_command, command.id, {:exit, 0}}, completion_ms)

      {:error, output, exit_code} ->
        Process.send_after(self(), {:emit_output, command.id, :stderr, output}, 10)
        Process.send_after(self(), {:complete_command, command.id, {:failed, exit_code, ""}}, 30)
    end

    {:noreply, state}
  end

  def handle_info({:complete_command, command_id, status}, state) do
    commands = Map.delete(state.commands, command_id)

    case status do
      {:exit, exit_code} ->
        notify(state, {:command_finished, state.sandbox_id, command_id, :exited, %{exit_code: exit_code}})

      {:failed, exit_code, message} ->
        notify(state, {:command_output, state.sandbox_id, command_id, :stderr, message})
        notify(state, {:command_finished, state.sandbox_id, command_id, :failed, %{exit_code: exit_code}})

      {:killed, signal} ->
        notify(state, {:command_finished, state.sandbox_id, command_id, :killed, %{signal: signal}})
      end

    {:noreply, %{state | commands: commands}}
  end

  @impl true
  def handle_call({:start_command, command}, _from, state) do
    command_id = command.id
    notify(state, {:command_started, state.sandbox_id, command_id, mock_pid(command_id)})

    schedule_command(command)

    {:reply, :ok, %{state | commands: Map.put(state.commands, command_id, command)}}
  end

  def handle_call({:send_stdin, command_id, data}, _from, state) do
    if Map.has_key?(state.commands, command_id) do
      notify(state, {:command_stdin, state.sandbox_id, command_id, data})
      Process.send_after(self(), {:emit_output, command_id, :stdout, "stdin> " <> data}, 5)
      {:reply, :ok, state}
    else
      {:reply, {:error, :command_not_found}, state}
    end
  end

  def handle_call({:cancel_command, command_id, opts}, _from, state) do
    if Map.has_key?(state.commands, command_id) do
      grace_period_ms = Map.get(opts, :grace_period_ms, 5_000)
      notify(state, {:command_cancelling, state.sandbox_id, command_id, grace_period_ms})
      Process.send_after(self(), {:complete_command, command_id, {:killed, "SIGKILL"}}, min(grace_period_ms, 50))
      {:reply, :ok, state}
    else
      {:reply, {:error, :command_not_found}, state}
    end
  end

  def handle_call({:kill_command, command_id}, _from, state) do
    if Map.has_key?(state.commands, command_id) do
      notify(state, {:command_kill_requested, state.sandbox_id, command_id})
      Process.send_after(self(), {:complete_command, command_id, {:killed, "SIGKILL"}}, 5)
      {:reply, :ok, state}
    else
      {:reply, {:error, :command_not_found}, state}
    end
  end

  defp schedule_command(command) do
    Process.send_after(self(), {:run_command, command}, 10)
  end

  defp command_output(state, command) do
    trimmed = String.trim(command.command)

    cond do
      trimmed == "pwd" ->
        {:ok, command.cwd <> "\n", 30}

      trimmed == "ls" or trimmed == "ls -l" or trimmed == "ls -la" or trimmed == "ls -latr" ->
        {:ok, list_workspace(state), 30}

      String.starts_with?(trimmed, "cat ") ->
        path = trimmed |> String.replace_prefix("cat ", "") |> String.trim()

        case Garden.Sandboxes.read_file(state.sandbox_id, normalize_path(path, command.cwd)) do
          {:ok, %{content: content}} -> {:ok, content, 30}
          _ -> {:error, "cat: #{path}: No such file or directory\n", 1}
        end

      String.starts_with?(trimmed, "echo ") ->
        {:ok, String.replace_prefix(trimmed, "echo ", "") <> "\n", 30}

      String.contains?(trimmed, "sleep") ->
        {:ok, "sleeping...\n", 120}

      String.contains?(trimmed, "fail") ->
        {:error, "mock failure\n", 1}

      true ->
        {:ok, "done\n", 30}
    end
  end

  defp list_workspace(state) do
    case Garden.Sandboxes.list_files(state.sandbox_id, "/workspace") do
      {:ok, %{entries: entries}} -> Enum.join(entries, "\n") <> "\n"
      _ -> "\n"
    end
  end

  defp normalize_path("/" <> _ = path, _cwd), do: path
  defp normalize_path(path, cwd), do: String.trim_trailing(cwd, "/") <> "/" <> path

  defp notify(state, message) do
    GenServer.cast(state.store, message)
  end

  defp via(sandbox_id), do: {:global, {__MODULE__, sandbox_id}}

  defp mock_pid(command_id) do
    :erlang.phash2(command_id, 65_535)
  end
end
