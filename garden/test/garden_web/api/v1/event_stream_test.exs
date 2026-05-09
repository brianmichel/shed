defmodule GardenWeb.Api.V1.EventStreamTest do
  use ExUnit.Case, async: true

  alias GardenWeb.Api.V1.EventStream

  test "encodes sse event payload" do
    event = %{
      id: "evt_1",
      seq: 1,
      cursor: "1",
      type: "command.stdout",
      sandbox_id: "sbx_1",
      command_id: "cmd_1",
      timestamp: "2026-05-08T00:00:00Z",
      data: %{"chunk" => "hello\n"}
    }

    encoded = EventStream.encode_event(event)

    assert String.starts_with?(encoded, "id: 1\nevent: command.stdout\n")
    assert String.contains?(encoded, ~s("command_id":"cmd_1"))
    assert String.ends_with?(encoded, "\n\n")
  end
end
