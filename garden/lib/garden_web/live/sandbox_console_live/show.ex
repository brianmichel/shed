defmodule GardenWeb.SandboxConsoleLive.Show do
  use GardenWeb, :live_view

  alias Garden.Sandboxes

  @impl true
  def mount(%{"sandbox_id" => sandbox_id}, _session, socket) do
    if connected?(socket) do
      Phoenix.PubSub.subscribe(Garden.PubSub, Sandboxes.sandbox_topic(sandbox_id))
    end

    {:ok, load(socket, sandbox_id)}
  end

  @impl true
  def handle_event("run_command", %{"command" => command}, socket) do
    case Sandboxes.start_command(socket.assigns.sandbox.id, %{"command" => command, "stdin" => true}) do
      {:ok, cmd} ->
        if connected?(socket), do: Phoenix.PubSub.subscribe(Garden.PubSub, Sandboxes.command_topic(socket.assigns.sandbox.id, cmd.id))
        {:noreply, refresh_commands(assign(socket, command_input: ""))}

      {:error, reason} -> {:noreply, put_flash(socket, :error, inspect(reason))}
    end
  end

  def handle_event("send_stdin", %{"command_id" => command_id, "data" => data}, socket) do
    _ = Sandboxes.send_stdin(socket.assigns.sandbox.id, command_id, %{"data" => data})
    {:noreply, socket}
  end

  def handle_event("cancel", %{"command_id" => command_id}, socket) do
    _ = Sandboxes.cancel_command(socket.assigns.sandbox.id, command_id, %{"grace_period_ms" => 100})
    {:noreply, socket}
  end

  def handle_event("kill", %{"command_id" => command_id}, socket) do
    _ = Sandboxes.kill_command(socket.assigns.sandbox.id, command_id, %{})
    {:noreply, socket}
  end

  def handle_event("read_file", %{"path" => path}, socket) do
    file_content =
      case Sandboxes.read_file(socket.assigns.sandbox.id, path) do
        {:ok, %{content: content}} -> content
        _ -> ""
      end

    {:noreply, assign(socket, selected_path: path, file_content: file_content)}
  end

  def handle_event("write_file", %{"path" => path, "content" => content}, socket) do
    case Sandboxes.write_file(socket.assigns.sandbox.id, path, content) do
      {:ok, _} -> {:noreply, load(assign(socket, selected_path: path, file_content: content), socket.assigns.sandbox.id)}
      {:error, reason} -> {:noreply, put_flash(socket, :error, inspect(reason))}
    end
  end

  @impl true
  def handle_info({:event, event}, socket) do
    socket =
      socket
      |> maybe_append_event(event)
      |> refresh_commands()

    {:noreply, socket}
  end

  defp load(socket, sandbox_id) do
    {:ok, _} = Sandboxes.ensure_sandbox(sandbox_id)
    sandbox = case Sandboxes.get_sandbox(sandbox_id) do {:ok, sbx} -> sbx; _ -> nil end
    commands = case Sandboxes.list_commands(sandbox_id) do {:ok, cmds} -> cmds; _ -> [] end
    files = case Sandboxes.list_files(sandbox_id, "/workspace") do {:ok, %{entries: entries}} -> entries; _ -> [] end

    if connected?(socket) do
      Enum.each(commands, fn cmd -> Phoenix.PubSub.subscribe(Garden.PubSub, Sandboxes.command_topic(sandbox_id, cmd.id)) end)
    end

    assign(socket,
      sandbox: sandbox,
      commands: commands,
      events: build_events(commands, sandbox_id),
      files: files,
      selected_path: socket.assigns[:selected_path] || "/workspace/README.txt",
      file_content: current_file_content(sandbox_id, socket.assigns[:selected_path] || "/workspace/README.txt"),
      command_input: ""
    )
  end

  defp refresh_commands(socket), do: load(socket, socket.assigns.sandbox.id)

  defp build_events(commands, sandbox_id) do
    Map.new(commands, fn cmd ->
      events = case Sandboxes.list_command_events(sandbox_id, cmd.id, %{}) do {:ok, %{events: events}} -> events; _ -> [] end
      {cmd.id, events}
    end)
  end

  defp current_file_content(sandbox_id, path) do
    case Sandboxes.read_file(sandbox_id, path) do
      {:ok, %{content: content}} -> content
      _ -> ""
    end
  end

  defp maybe_append_event(socket, %{command_id: command_id} = event) when not is_nil(command_id) do
    update(socket, :events, fn events -> Map.update(events || %{}, command_id, [event], &(&1 ++ [event])) end)
  end

  defp maybe_append_event(socket, _event), do: socket

  @impl true
  def render(assigns) do
    ~H"""
    <Layouts.app flash={@flash}>
      <div :if={@sandbox} class="max-w-7xl mx-auto space-y-6">
        <div class="flex items-center justify-between">
          <div>
            <.link navigate={~p"/seed/sessions"} class="link link-hover text-sm">← Sessions</.link>
            <h1 class="text-2xl font-semibold mt-2">Sandbox console</h1>
            <p class="font-mono text-sm">{@sandbox.id} · {@sandbox.state}</p>
            <p class="text-sm text-base-content/70">Backend: <span class="badge badge-secondary">{@sandbox.metadata["backend"] || @sandbox.capabilities["backend"] || "unknown"}</span></p>
          </div>
        </div>

        <div class="grid lg:grid-cols-2 gap-6">
          <div class="card bg-base-200 p-4 space-y-4">
            <h2 class="font-semibold">Commands</h2>
            <.form for={%{}} phx-submit="run_command" class="flex gap-2">
              <input name="command" value={@command_input} class="input input-bordered w-full" placeholder="echo hello" />
              <button class="btn btn-primary">Run</button>
            </.form>

            <div class="space-y-4 max-h-[32rem] overflow-auto">
              <div :for={cmd <- @commands} class="rounded bg-base-100 p-3 border border-base-300 space-y-2">
                <div class="flex items-center justify-between gap-3">
                  <div>
                    <div class="font-mono text-xs">{cmd.id}</div>
                    <div>{cmd.command}</div>
                  </div>
                  <div class="flex items-center gap-2">
                    <span class="badge">{cmd.state}</span>
                    <button class="btn btn-xs" phx-click="cancel" phx-value-command_id={cmd.id}>Cancel</button>
                    <button class="btn btn-xs btn-error" phx-click="kill" phx-value-command_id={cmd.id}>Kill</button>
                  </div>
                </div>
                <pre class="text-xs whitespace-pre-wrap max-h-40 overflow-auto">{render_events(@events[cmd.id] || [])}</pre>
                <.form for={%{}} phx-submit="send_stdin" class="flex gap-2">
                  <input type="hidden" name="command_id" value={cmd.id} />
                  <input name="data" class="input input-bordered w-full input-sm" placeholder="stdin" />
                  <button class="btn btn-sm">Send</button>
                </.form>
              </div>
            </div>
          </div>

          <div class="card bg-base-200 p-4 space-y-4">
            <h2 class="font-semibold">Files</h2>
            <div class="flex gap-2 flex-wrap">
              <button :for={entry <- @files} class="btn btn-xs" phx-click="read_file" phx-value-path={"/workspace/" <> entry}>{entry}</button>
            </div>
            <.form for={%{}} phx-submit="write_file" class="space-y-2">
              <input name="path" value={@selected_path} class="input input-bordered w-full font-mono" />
              <textarea name="content" class="textarea textarea-bordered w-full h-80 font-mono">{@file_content}</textarea>
              <button class="btn btn-primary">Write file</button>
            </.form>
          </div>
        </div>
      </div>
    </Layouts.app>
    """
  end

  defp render_events(events) do
    events
    |> Enum.map(fn event ->
      chunk = get_in(event, [:data, "chunk"])
      cond do
        chunk -> chunk
        event.type == "command.exit" -> "\n[exit #{get_in(event, [:data, "exit_code"]) || 0}]\n"
        event.type == "command.killed" -> "\n[killed]\n"
        true -> ""
      end
    end)
    |> Enum.join()
  end
end
