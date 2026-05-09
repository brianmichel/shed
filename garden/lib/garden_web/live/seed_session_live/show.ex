defmodule GardenWeb.SeedSessionLive.Show do
  use GardenWeb, :live_view

  alias Garden.SeedSessions
  alias Garden.SeedSimulator

  @impl true
  def mount(%{"session_id" => session_id}, _session, socket) do
    if connected?(socket) do
      Phoenix.PubSub.subscribe(Garden.PubSub, SeedSessions.topic(session_id))
    end

    {:ok, load_session(socket, session_id)}
  end

  @impl true
  def handle_event("start_simulator", _params, socket) do
    case SeedSimulator.start_for_session(socket.assigns.session.session_id) do
      {:ok, _pid} -> {:noreply, socket}
      {:error, {:already_started, _pid}} -> {:noreply, socket}
      _ -> {:noreply, put_flash(socket, :error, "Could not start simulator")}
    end
  end

  @impl true
  def handle_event("send_message", %{"type" => type}, socket) do
    session = socket.assigns.session

    payload =
      case type do
        "garden.hello" ->
          %{
            "protocol_version" => "1",
            "session_id" => session.session_id,
            "sandbox_id" => session.sandbox_id,
            "heartbeat_interval_ms" => 10_000,
            "ack_timeout_ms" => 5_000
          }

        "garden.lease_warning" ->
          %{"lease_expires_at" => DateTime.to_iso8601(DateTime.add(DateTime.utc_now(), 300, :second)), "remaining_ms" => 300_000}

        "garden.shutdown" ->
          %{"reason" => "operator_requested", "deadline" => DateTime.to_iso8601(DateTime.add(DateTime.utc_now(), 30, :second))}
      end

    {:ok, _message} = SeedSessions.dispatch(session.session_id, type, payload)
    {:noreply, socket}
  end

  @impl true
  def handle_info({:session_updated, session}, socket) do
    {:noreply, assign(socket, session: session)}
  end

  def handle_info({:garden_message, _message}, socket), do: {:noreply, socket}
  def handle_info({:seed_message, _message}, socket), do: {:noreply, socket}

  defp load_session(socket, session_id) do
    case SeedSessions.get(session_id) do
      {:ok, session} -> assign(socket, session: session)
      {:error, _} -> socket |> put_flash(:error, "Session not found") |> push_navigate(to: ~p"/seed/sessions")
    end
  end

  @impl true
  def render(assigns) do
    ~H"""
    <Layouts.app flash={@flash}>
      <div class="space-y-6 max-w-6xl mx-auto">
        <div class="flex items-center justify-between">
          <div>
            <.link navigate={~p"/seed/sessions"} class="link link-hover text-sm">← Back to sessions</.link>
            <h1 class="text-2xl font-semibold mt-2">{assigns.session.session_id}</h1>
            <p class="text-sm text-base-content/70 font-mono">sandbox: {assigns.session.sandbox_id}</p>
            <p class="text-sm text-base-content/70">Current backend: <span class="badge badge-secondary">{Garden.SandboxBackend.name()}</span></p>
          </div>
          <div class="flex gap-2">
            <.link navigate={~p"/sandboxes/#{@session.sandbox_id}/console"} class="btn btn-sm">Open sandbox console</.link>
            <button class="btn btn-sm btn-secondary" phx-click="start_simulator">Start simulator</button>
            <button class="btn btn-sm" phx-click="send_message" phx-value-type="garden.hello">Send garden.hello</button>
            <button class="btn btn-sm btn-warning" phx-click="send_message" phx-value-type="garden.lease_warning">Send lease warning</button>
            <button class="btn btn-sm btn-error" phx-click="send_message" phx-value-type="garden.shutdown">Send shutdown</button>
          </div>
        </div>

        <div class="grid md:grid-cols-4 gap-4">
          <div class="card bg-base-200 p-4"><div class="text-xs opacity-70">Status</div><div class="font-semibold">{@session.status}</div></div>
          <div class="card bg-base-200 p-4"><div class="text-xs opacity-70">Seed seq</div><div class="font-semibold">{@session.last_seed_seq_seen}</div></div>
          <div class="card bg-base-200 p-4"><div class="text-xs opacity-70">Garden seq</div><div class="font-semibold">{@session.last_garden_seq_sent}</div></div>
          <div class="card bg-base-200 p-4"><div class="text-xs opacity-70">Messages</div><div class="font-semibold">{length(@session.messages)}</div></div>
        </div>

        <div class="card bg-base-200 p-4 space-y-3">
          <h2 class="font-semibold">Connection details</h2>
          <div class="grid md:grid-cols-2 gap-4 text-sm">
            <div><span class="opacity-70">Session key:</span> <span class="font-mono break-all">{@session.session_key}</span></div>
            <div><span class="opacity-70">Socket connected:</span> {if @session.socket_pid, do: "yes", else: "no"}</div>
          </div>
        </div>

        <div class="card bg-base-200 p-4">
          <h2 class="font-semibold mb-3">Protocol messages</h2>
          <div class="space-y-3 max-h-[34rem] overflow-auto">
            <div :for={entry <- Enum.reverse(@session.messages)} class="rounded border border-base-300 bg-base-100 p-3">
              <div class="flex items-center justify-between gap-3 mb-2">
                <span class={["badge badge-sm", entry.direction == "outbound" && "badge-primary", entry.direction == "inbound" && "badge-accent"]}>{entry.direction}</span>
                <span class="font-mono text-xs">{entry.message["type"]} · seq {entry.message["seq"]}</span>
              </div>
              <pre class="text-xs overflow-auto whitespace-pre-wrap">{Jason.encode_to_iodata!(entry.message, pretty: true)}</pre>
            </div>
          </div>
        </div>
      </div>
    </Layouts.app>
    """
  end
end
