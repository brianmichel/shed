defmodule GardenWeb.Api.V1.EventStream do
  @moduledoc false

  import Plug.Conn

  @keepalive_ms 15_000

  def encode_event(event) do
    payload = Jason.encode!(event)
    ["id: ", event.cursor, "\n", "event: ", event.type, "\n", "data: ", payload, "\n\n"]
    |> IO.iodata_to_binary()
  end

  def stream(conn, events, topic) do
    conn =
      conn
      |> put_resp_header("cache-control", "no-cache")
      |> put_resp_header("connection", "keep-alive")
      |> put_resp_content_type("text/event-stream")
      |> send_chunked(:ok)

    Phoenix.PubSub.subscribe(Garden.PubSub, topic)

    with {:ok, conn} <- write_events(conn, events) do
      loop(conn)
    end
  end

  defp loop(conn) do
    receive do
      {:event, event} ->
        case write_event(conn, event) do
          {:ok, conn} -> loop(conn)
          {:error, _} -> conn
        end
    after
      @keepalive_ms ->
        case chunk(conn, ": keepalive\n\n") do
          {:ok, conn} -> loop(conn)
          {:error, _} -> conn
        end
    end
  end

  defp write_events(conn, events) do
    Enum.reduce_while(events, {:ok, conn}, fn event, {:ok, conn} ->
      case write_event(conn, event) do
        {:ok, conn} -> {:cont, {:ok, conn}}
        {:error, reason} -> {:halt, {:error, reason}}
      end
    end)
  end

  defp write_event(conn, event) do
    chunk(conn, encode_event(event))
  end
end
