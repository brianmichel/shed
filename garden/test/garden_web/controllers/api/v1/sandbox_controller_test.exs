defmodule GardenWeb.Api.V1.SandboxControllerTest do
  use GardenWeb.ConnCase, async: false

  alias Garden.Sandboxes.Store

  setup do
    Store.reset!()
    :ok
  end

  test "acquires a sandbox and replays sandbox events", %{conn: conn} do
    conn =
      post(conn, ~p"/api/v1/sandboxes", %{
        environment: "linux",
        template: "ubuntu-dev",
        lease: %{ttl_ms: 1_000},
        metadata: %{session_id: "sess_123"}
      })

    assert %{"data" => sandbox, "operation" => operation} = json_response(conn, 201)
    assert sandbox["state"] == "provisioning"
    assert operation["state"] == "pending"

    Process.sleep(40)

    conn = get(build_conn(), ~p"/api/v1/sandboxes/#{sandbox["id"]}")
    assert %{"data" => shown} = json_response(conn, 200)
    assert shown["state"] == "ready"

    conn = get(build_conn(), ~p"/api/v1/sandboxes/#{sandbox["id"]}/events")
    assert %{"data" => events} = json_response(conn, 200)
    assert Enum.any?(events, &(&1["type"] == "sandbox.provisioning"))
    assert Enum.any?(events, &(&1["type"] == "sandbox.booting"))
    assert Enum.any?(events, &(&1["type"] == "sandbox.ready"))
  end

  test "rejects invalid lease payload", %{conn: conn} do
    conn = post(conn, ~p"/api/v1/sandboxes", %{})
    sandbox = json_response(conn, 201)["data"]

    conn = post(build_conn(), ~p"/api/v1/sandboxes/#{sandbox["id"]}/lease", %{ttl_ms: 0})
    assert %{"error" => error} = json_response(conn, 422)
    assert error["code"] == "invalid_request"
    assert error["details"]["ttl_ms"] == ["must be greater than 0"]
  end

  test "extends a lease and releases a sandbox", %{conn: conn} do
    conn = post(conn, ~p"/api/v1/sandboxes", %{})
    sandbox = json_response(conn, 201)["data"]

    conn = post(build_conn(), ~p"/api/v1/sandboxes/#{sandbox["id"]}/lease", %{ttl_ms: 5_000, reason: "test"})
    assert %{"data" => lease} = json_response(conn, 200)
    assert lease["ttl_ms"] == 5_000

    conn = post(build_conn(), ~p"/api/v1/sandboxes/#{sandbox["id"]}/release", %{reason: "done"})
    assert %{"data" => released} = json_response(conn, 200)
    assert released["state"] == "released"
  end
end
