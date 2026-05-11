defmodule GardenWeb.SeedSessionLive.Index do
  use GardenWeb, :live_view

  alias Garden.Sandboxes
  alias Garden.SeedSessions
  alias Garden.SeedSimulator

  @impl true
  def mount(_params, _session, socket) do
    if connected?(socket) do
      Phoenix.PubSub.subscribe(Garden.PubSub, SeedSessions.index_topic())
    end

    {:ok, assign(socket, sessions: SeedSessions.list_sessions(), sandbox_id: random_sandbox_id())}
  end

  @impl true
  def handle_event("issue_session", %{"sandbox_id" => sandbox_id}, socket) do
    sandbox_id = if String.trim(sandbox_id) == "", do: random_sandbox_id(), else: sandbox_id
    {:ok, _sandbox} = Sandboxes.ensure_sandbox(sandbox_id)
    # ensure_sandbox calls setup_sandbox for LocalHost which issues a session internally;
    # only issue explicitly if no active session exists (mock backend path)
    case SeedSessions.find_by_sandbox(sandbox_id) do
      {:ok, %{status: status}} when status in [:issued, :connected] -> :ok
      _ -> SeedSessions.issue_session(sandbox_id)
    end
    {:noreply, assign(socket, sandbox_id: random_sandbox_id())}
  end

  @impl true
  def handle_event("start_simulator", %{"session_id" => session_id}, socket) do
    case SeedSimulator.start_for_session(session_id) do
      {:ok, _pid} -> {:noreply, socket}
      {:error, {:already_started, _pid}} -> {:noreply, socket}
      _ -> {:noreply, put_flash(socket, :error, "Could not start simulator")}
    end
  end

  @impl true
  def handle_info({:session_updated, _session}, socket) do
    {:noreply, assign(socket, sessions: SeedSessions.list_sessions())}
  end

  @adjectives ~w(monke brain cool soft bright fuzzy cosmic sleepy brave lucky tiny mellow rapid amber silver velvet neon)
  @nouns ~w(touch river cloud stone forest comet mango circuit ember meadow shadow signal rocket breeze lantern)

  defp random_sandbox_id do
    [a, b] = Enum.take_random(@adjectives, 2)
    noun = Enum.random(@nouns)
    Enum.join([a, b, noun], "-")
  end

  @impl true
  def render(assigns) do
    ~H"""
    <Layouts.app flash={@flash}>
      <div class="space-y-6 max-w-5xl mx-auto">
        <div class="flex items-center justify-between gap-4">
          <div>
            <h1 class="text-2xl font-semibold">Seed sessions</h1>
            <p class="text-sm text-base-content/70">Issue demo Seed sessions and inspect live protocol traffic.</p>
          </div>
          <.link navigate={~p"/seed/sessions"} class="btn btn-ghost btn-sm">Refresh</.link>
        </div>

        <.form for={%{}} phx-submit="issue_session" class="card bg-base-200 p-4">
          <div class="flex gap-3 items-end">
            <div class="flex-1">
              <label class="label"><span class="label-text">Sandbox ID</span></label>
              <input name="sandbox_id" value={@sandbox_id} class="input input-bordered w-full" />
            </div>
            <button class="btn btn-primary">Issue session</button>
          </div>
        </.form>

        <div class="overflow-x-auto card bg-base-200">
          <table class="table">
            <thead>
              <tr>
                <th>Session</th>
                <th>Sandbox</th>
                <th>Status</th>
                <th>Seed seq</th>
                <th>Garden seq</th>
                <th>Messages</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              <tr :for={session <- @sessions}>
                <td class="font-mono text-xs">{session.session_id}</td>
                <td class="font-mono text-xs">{session.sandbox_id}</td>
                <td><span class="badge">{session.status}</span></td>
                <td>{session.last_seed_seq_seen}</td>
                <td>{session.last_garden_seq_sent}</td>
                <td>{length(session.messages)}</td>
                <td class="space-x-2">
                  <button phx-click="start_simulator" phx-value-session_id={session.session_id} class="btn btn-xs btn-secondary">Simulate</button>
                  <.link navigate={~p"/seed/sessions/#{session.session_id}"} class="btn btn-sm">Open</.link>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </Layouts.app>
    """
  end
end
