defmodule GardenWeb.SeedChannel do
  use GardenWeb, :channel

  alias Garden.SeedProtocol.Message
  alias Garden.SeedSessions

  @impl true
  def join("sandbox:" <> sandbox_id, _payload, socket) do
    if sandbox_id == socket.assigns.sandbox_id do
      Phoenix.PubSub.subscribe(Garden.PubSub, topic(socket.assigns.session_id))
      {:ok, %{session_id: socket.assigns.session_id, sandbox_id: sandbox_id}, socket}
    else
      {:error, %{reason: "sandbox_mismatch"}}
    end
  end

  @impl true
  def handle_in("message", params, socket) do
    case Message.validate(params) do
      {:ok, message} ->
        cond do
          message.session_id != socket.assigns.session_id ->
            {:reply, {:error, %{error: protocol_error("session_mismatch")}}, socket}

          message.sandbox_id != socket.assigns.sandbox_id ->
            {:reply, {:error, %{error: protocol_error("sandbox_mismatch")}}, socket}

          true ->
            case SeedSessions.record_inbound(socket.assigns.session_id, message) do
              {:ok, _session} ->
                maybe_ack(socket, message)

              {:error, :duplicate_message} ->
                {:reply, {:ok, %{duplicate: true}}, socket}

              {:error, reason} ->
                {:reply, {:error, %{error: protocol_error(to_string(reason))}}, socket}
            end
        end

      {:error, changeset} ->
        {:reply, {:error, %{error: protocol_error("invalid_message", %{details: traverse_errors(changeset)})}}, socket}
    end
  end

  @impl true
  def handle_info({:garden_message, message}, socket) do
    push(socket, "message", message)
    {:noreply, socket}
  end

  def handle_info({:seed_message, _message}, socket) do
    {:noreply, socket}
  end

  def handle_info({:session_updated, _session}, socket) do
    {:noreply, socket}
  end

  @impl true
  def terminate(_reason, socket) do
    SeedSessions.disconnect(socket.assigns.session_id)
    :ok
  end

  defp maybe_ack(socket, %{expects_ack: true} = message) do
    {:ok, ack} =
      SeedSessions.dispatch(socket.assigns.session_id, "ack", %{"status" => "accepted"},
        ack_id: message.message_id,
        request_id: message.request_id,
        expects_ack: false,
        reply_to: message.message_id
      )

    {:reply, {:ok, Message.to_map(ack)}, socket}
  end

  defp maybe_ack(socket, _message), do: {:noreply, socket}

  defp topic(session_id), do: "seed_sessions:" <> session_id

  defp protocol_error(code, extra \\ %{}) do
    Map.merge(
      %{
        code: code,
        message: "Protocol error",
        retryable: false
      },
      extra
    )
  end

  defp traverse_errors(changeset) do
    Ecto.Changeset.traverse_errors(changeset, fn {message, opts} ->
      Regex.replace(~r/%{(\w+)}/, message, fn _, key ->
        opts |> Keyword.get(String.to_existing_atom(key), key) |> to_string()
      end)
    end)
  end
end
