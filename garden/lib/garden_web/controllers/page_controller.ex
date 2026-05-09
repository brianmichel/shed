defmodule GardenWeb.PageController do
  use GardenWeb, :controller

  def home(conn, _params) do
    render(conn, :home)
  end
end
