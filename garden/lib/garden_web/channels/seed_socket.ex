defmodule GardenWeb.SeedSocket do
  use Phoenix.Socket

  alias Garden.SeedSessions

  channel "sandbox:*", GardenWeb.SeedChannel

  @impl true
  def connect(%{"session_key" => session_key, "sandbox_id" => sandbox_id}, socket, _connect_info) do
    with {:ok, session} <- SeedSessions.authenticate(session_key, sandbox_id),
         {:ok, _session} <- SeedSessions.connect(session.session_id, self()) do
      socket =
        socket
        |> assign(:session_id, session.session_id)
        |> assign(:sandbox_id, sandbox_id)
        |> assign(:session_key, session_key)

      {:ok, socket}
    else
      _ -> :error
    end
  end

  def connect(_, _socket, _connect_info), do: :error

  @impl true
  def id(socket), do: "seed_socket:" <> socket.assigns.session_id
end
