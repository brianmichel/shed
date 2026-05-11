defmodule GardenWeb.SeedHandler do
  @behaviour WebSock

  alias Garden.SeedProtocol.Message
  alias Garden.SeedSessions

  @impl WebSock
  def init(%{session_id: session_id, sandbox_id: sandbox_id}) do
    Phoenix.PubSub.subscribe(Garden.PubSub, "seed_sessions:" <> session_id)
    {:ok, %{session_id: session_id, sandbox_id: sandbox_id}}
  end

  @impl WebSock
  def handle_in({data, opcode: :text}, state) do
    case Jason.decode(data) do
      {:ok, params} -> handle_message(params, state)
      {:error, _} -> {:reply, :ok, {:text, error_json("invalid_json", "Could not parse message")}, state}
    end
  end

  def handle_in({_data, opcode: :binary}, state) do
    {:reply, :ok, {:text, error_json("unsupported", "Binary frames are not supported")}, state}
  end

  @impl WebSock
  def handle_info({:garden_message, message}, state) do
    {:push, {:text, Jason.encode!(message)}, state}
  end

  def handle_info({:seed_message, _message}, state), do: {:ok, state}
  def handle_info({:session_updated, _session}, state), do: {:ok, state}

  @impl WebSock
  def terminate(_reason, state) do
    SeedSessions.disconnect(state.session_id)
    :ok
  end

  defp handle_message(params, state) do
    case Message.validate(params) do
      {:ok, message} ->
        cond do
          message.session_id != state.session_id ->
            {:reply, :ok, {:text, error_json("session_mismatch", "Session ID does not match")}, state}

          message.sandbox_id != state.sandbox_id ->
            {:reply, :ok, {:text, error_json("sandbox_mismatch", "Sandbox ID does not match")}, state}

          true ->
            case SeedSessions.record_inbound(state.session_id, message) do
              {:ok, _session} ->
                maybe_ack(message, state)

              {:error, :duplicate_message} ->
                {:reply, :ok, {:text, Jason.encode!(%{duplicate: true})}, state}

              {:error, reason} ->
                {:reply, :ok, {:text, error_json(to_string(reason), "Protocol error")}, state}
            end
        end

      {:error, changeset} ->
        details = traverse_errors(changeset)
        {:reply, :ok, {:text, error_json("invalid_message", "Validation failed", %{details: details})}, state}
    end
  end

  defp maybe_ack(%{expects_ack: true} = message, state) do
    {:ok, ack} =
      SeedSessions.dispatch(state.session_id, "ack", %{"status" => "accepted"},
        ack_id: message.message_id,
        request_id: message.request_id,
        expects_ack: false,
        reply_to: message.message_id
      )

    {:push, {:text, Jason.encode!(Message.to_map(ack))}, state}
  end

  defp maybe_ack(_message, state), do: {:ok, state}

  defp error_json(code, message, extra \\ %{}) do
    Jason.encode!(Map.merge(%{"error" => %{"code" => code, "message" => message}}, extra))
  end

  defp traverse_errors(changeset) do
    Ecto.Changeset.traverse_errors(changeset, fn {message, opts} ->
      Regex.replace(~r/%{(\w+)}/, message, fn _, key ->
        opts |> Keyword.get(String.to_existing_atom(key), key) |> to_string()
      end)
    end)
  end
end
