defmodule GardenWeb.SeedChannelTest do
  use GardenWeb.ChannelCase, async: false

  alias Garden.SeedSessions
  alias Garden.SeedSessions.Store

  setup do
    Store.reset!()
    {:ok, session} = SeedSessions.issue_session("sbx_123")

    {:ok, socket} =
      connect(GardenWeb.SeedSocket, %{
        "session_key" => session.session_key,
        "sandbox_id" => session.sandbox_id
      })

    {:ok, _, socket} = subscribe_and_join(socket, GardenWeb.SeedChannel, "sandbox:sbx_123")

    %{socket: socket, session: session}
  end

  test "acks valid inbound messages", %{socket: socket, session: session} do
    ref = push(socket, "message", %{
      "version" => "1",
      "type" => "seed.heartbeat",
      "message_id" => "msg_1",
      "request_id" => "req_1",
      "session_id" => session.session_id,
      "sandbox_id" => session.sandbox_id,
      "seq" => 1,
      "timestamp" => "2026-05-08T22:00:00Z",
      "expects_ack" => true,
      "payload" => %{
        "uptime_ms" => 1000,
        "active_commands" => 0,
        "last_garden_seq_seen" => 0,
        "last_seed_seq_sent" => 1,
        "connection_generation" => 1
      }
    })

    assert_reply ref, :ok, %{"type" => "ack", "ack_id" => "msg_1"}
  end

  test "dispatch pushes outbound protocol messages", %{socket: socket, session: session} do
    {:ok, dispatched} =
      SeedSessions.dispatch(session.session_id, "garden.lease_warning", %{
        "lease_expires_at" => "2026-05-08T22:10:00Z",
        "remaining_ms" => 10_000
      })

    assert_receive %Phoenix.Socket.Message{event: "message", payload: %{"type" => "garden.lease_warning", "message_id" => message_id}}
    assert message_id == dispatched.message_id
    assert socket.assigns.session_id == session.session_id
  end
end
