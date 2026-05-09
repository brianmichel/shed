defmodule GardenWeb.OpenAPIController do
  use GardenWeb, :controller

  def show(conn, _params) do
    json(conn, GardenWeb.OpenAPI.spec())
  end
end
