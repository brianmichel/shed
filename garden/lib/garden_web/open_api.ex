defmodule GardenWeb.OpenAPI do
  @moduledoc false

  def spec, do: GardenWeb.GeneratedOpenAPI.spec()
end
