defmodule GardenWeb.OpenAPIControllerTest do
  use GardenWeb.ConnCase, async: true

  test "serves openapi spec", %{conn: conn} do
    conn = get(conn, ~p"/api/openapi.json")
    assert %{"openapi" => "3.1.0", "paths" => paths} = json_response(conn, 200)
    assert Map.has_key?(paths, "/api/v1/sandboxes")
  end
end
