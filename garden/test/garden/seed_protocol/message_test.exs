defmodule Garden.SeedProtocol.MessageTest do
  use ExUnit.Case, async: true

  alias Garden.SeedProtocol.Message

  test "validates a command.start envelope and payload" do
    params = %{
      "version" => "1",
      "type" => "command.start",
      "message_id" => "msg_1",
      "request_id" => "req_1",
      "session_id" => "sess_1",
      "sandbox_id" => "sbx_1",
      "seq" => 1,
      "timestamp" => "2026-05-08T22:00:00Z",
      "expects_ack" => true,
      "payload" => %{
        "command_id" => "cmd_1",
        "command" => "echo hello"
      }
    }

    assert {:ok, message} = Message.validate(params)
    assert message.type == "command.start"
    assert message.payload["command_id"] == "cmd_1"
  end

  test "rejects invalid payloads" do
    params = %{
      "version" => "1",
      "type" => "command.cancel",
      "message_id" => "msg_1",
      "request_id" => "req_1",
      "session_id" => "sess_1",
      "sandbox_id" => "sbx_1",
      "seq" => 1,
      "timestamp" => "2026-05-08T22:00:00Z",
      "payload" => %{
        "command_id" => "cmd_1",
        "escalation" => "nope"
      }
    }

    assert {:error, changeset} = Message.validate(params)
    assert "is invalid" in errors_on(changeset).payload
  end

  defp errors_on(changeset) do
    Ecto.Changeset.traverse_errors(changeset, fn {message, _opts} -> message end)
  end
end
