defmodule GardenWeb.ChannelCase do
  use ExUnit.CaseTemplate

  using do
    quote do
      import Phoenix.ChannelTest
      @endpoint GardenWeb.Endpoint
    end
  end
end
