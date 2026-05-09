defmodule GardenWeb.Api.V1.CommandControllerTest do
  use GardenWeb.ConnCase, async: false

  alias Garden.Sandboxes.Store

  setup do
    Store.reset!()
    :ok
  end

  test "starts a command, streams durable events, and accepts stdin", %{conn: conn} do
    sandbox_id = acquire_ready_sandbox(conn)

    conn =
      post(build_conn(), ~p"/api/v1/sandboxes/#{sandbox_id}/commands", %{
        command: "echo hello",
        stdin: true
      })

    assert %{"data" => command} = json_response(conn, 201)

    conn = post(build_conn(), ~p"/api/v1/sandboxes/#{sandbox_id}/commands/#{command["id"]}/stdin", %{data: "hi\n"})
    assert %{"data" => %{"accepted" => true}} = json_response(conn, 200)

    Process.sleep(60)

    conn = get(build_conn(), ~p"/api/v1/sandboxes/#{sandbox_id}/commands/#{command["id"]}")
    assert %{"data" => shown} = json_response(conn, 200)
    assert shown["state"] == "exited"

    conn = get(build_conn(), ~p"/api/v1/sandboxes/#{sandbox_id}/commands/#{command["id"]}/events")
    assert %{"data" => events} = json_response(conn, 200)
    assert Enum.any?(events, &(&1["type"] == "command.queued"))
    assert Enum.any?(events, &(&1["type"] == "command.started"))
    assert Enum.any?(events, &(&1["type"] == "command.stdout"))
    assert Enum.any?(events, &(&1["type"] == "command.stdin.accepted"))
    assert Enum.any?(events, &(&1["type"] == "command.exit"))
  end

  test "rejects invalid command payload", %{conn: conn} do
    sandbox_id = acquire_ready_sandbox(conn)

    conn = post(build_conn(), ~p"/api/v1/sandboxes/#{sandbox_id}/commands", %{})
    assert %{"error" => error} = json_response(conn, 422)
    assert error["code"] == "invalid_request"
    assert error["details"]["command"] == ["can't be blank"]
  end

  test "cancels a running command", %{conn: conn} do
    sandbox_id = acquire_ready_sandbox(conn)

    conn =
      post(build_conn(), ~p"/api/v1/sandboxes/#{sandbox_id}/commands", %{
        command: "sleep 10"
      })

    command = json_response(conn, 201)["data"]

    Process.sleep(15)

    conn = post(build_conn(), ~p"/api/v1/sandboxes/#{sandbox_id}/commands/#{command["id"]}/cancel", %{grace_period_ms: 10})
    assert %{"data" => cancelling} = json_response(conn, 200)
    assert cancelling["state"] == "cancelling"

    Process.sleep(80)

    conn = get(build_conn(), ~p"/api/v1/sandboxes/#{sandbox_id}/commands/#{command["id"]}")
    assert %{"data" => shown} = json_response(conn, 200)
    assert shown["state"] == "killed"
  end

  defp acquire_ready_sandbox(conn) do
    conn = post(conn, ~p"/api/v1/sandboxes", %{})
    sandbox_id = json_response(conn, 201)["data"]["id"]
    Process.sleep(40)
    sandbox_id
  end
end
