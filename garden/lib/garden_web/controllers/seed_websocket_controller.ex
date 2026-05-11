defmodule GardenWeb.SeedWebSocketController do
  use GardenWeb, :controller

  alias Garden.SeedSessions

  def upgrade(conn, %{"session_key" => session_key, "sandbox_id" => sandbox_id}) do
    with {:ok, session} <- SeedSessions.authenticate(session_key, sandbox_id),
         {:ok, _session} <- SeedSessions.connect(session.session_id, self()) do
      conn
      |> WebSockAdapter.upgrade(GardenWeb.SeedHandler, %{
        session_id: session.session_id,
        sandbox_id: sandbox_id
      }, timeout: :infinity)
      |> halt()
    else
      _ ->
        conn
        |> put_status(401)
        |> json(%{error: %{code: "unauthorized", message: "Invalid session key or sandbox ID"}})
    end
  end

  def upgrade(conn, _params) do
    conn
    |> put_status(400)
    |> json(%{error: %{code: "missing_params", message: "session_key and sandbox_id are required"}})
  end
end
