defmodule GardenWeb.Api.V1.CommandController do
  use GardenWeb, :controller

  alias Garden.Sandboxes
  alias GardenWeb.Api.V1.EventStream
  alias GardenWeb.Api.V1.Schemas.{CommandCancelParams, CommandCreateParams, CommandKillParams, CommandStdinParams}
  alias GardenWeb.Api.V1.Validation

  def index(conn, params) do
    sandbox_id = sandbox_id!(params)

    case Sandboxes.list_commands(sandbox_id) do
      {:ok, commands} -> json(conn, %{data: commands})
      {:error, reason} -> render_error(conn, reason)
    end
  end

  def create(conn, params) do
    sandbox_id = sandbox_id!(params)

    with {:ok, attrs} <- Validation.validate(CommandCreateParams, params),
         {:ok, command} <- Sandboxes.start_command(sandbox_id, attrs) do
      conn
      |> put_status(:created)
      |> json(%{data: command})
    else
      {:error, reason} -> render_error(conn, reason)
    end
  end

  def show(conn, params) do
    sandbox_id = sandbox_id!(params)
    command_id = command_id!(params)

    case Sandboxes.get_command(sandbox_id, command_id) do
      {:ok, command} -> json(conn, %{data: command})
      {:error, reason} -> render_error(conn, reason)
    end
  end

  def stdin(conn, params) do
    sandbox_id = sandbox_id!(params)
    command_id = command_id!(params)

    with {:ok, attrs} <- Validation.validate(CommandStdinParams, params),
         {:ok, result} <- Sandboxes.send_stdin(sandbox_id, command_id, attrs) do
      json(conn, %{data: result})
    else
      {:error, reason} -> render_error(conn, reason)
    end
  end

  def cancel(conn, params) do
    sandbox_id = sandbox_id!(params)
    command_id = command_id!(params)

    with {:ok, attrs} <- Validation.validate(CommandCancelParams, params),
         {:ok, command} <- Sandboxes.cancel_command(sandbox_id, command_id, attrs) do
      json(conn, %{data: command})
    else
      {:error, reason} -> render_error(conn, reason)
    end
  end

  def kill(conn, params) do
    sandbox_id = sandbox_id!(params)
    command_id = command_id!(params)

    with {:ok, attrs} <- Validation.validate(CommandKillParams, params),
         {:ok, command} <- Sandboxes.kill_command(sandbox_id, command_id, attrs) do
      json(conn, %{data: command})
    else
      {:error, reason} -> render_error(conn, reason)
    end
  end

  def events(conn, params) do
    sandbox_id = sandbox_id!(params)
    command_id = command_id!(params)

    case Sandboxes.list_command_events(sandbox_id, command_id, params) do
      {:ok, %{events: events, next_cursor: next_cursor}} ->
        if sse?(conn) do
          EventStream.stream(conn, events, Sandboxes.command_topic(sandbox_id, command_id))
        else
          json(conn, %{data: events, next_cursor: next_cursor})
        end

      {:error, reason} ->
        render_error(conn, reason)
    end
  end

  defp sandbox_id!(params), do: Map.get(params, "sandbox_id") || Map.fetch!(params, "sandbox_sandbox_id")
  defp command_id!(params), do: Map.get(params, "command_id") || Map.fetch!(params, "command_command_id")

  defp sse?(conn) do
    conn
    |> get_req_header("accept")
    |> Enum.any?(&String.contains?(&1, "text/event-stream"))
  end

  defp render_error(conn, %{code: _} = error) do
    conn
    |> put_status(:unprocessable_entity)
    |> json(%{error: error})
  end

  defp render_error(conn, :sandbox_not_found) do
    conn
    |> put_status(:not_found)
    |> json(%{error: %{code: "sandbox_not_found", message: "Sandbox not found", retryable: false}})
  end

  defp render_error(conn, :command_not_found) do
    conn
    |> put_status(:not_found)
    |> json(%{error: %{code: "command_not_found", message: "Command not found", retryable: false}})
  end

  defp render_error(conn, :sandbox_not_ready) do
    conn
    |> put_status(:conflict)
    |> json(%{error: %{code: "sandbox_not_ready", message: "Sandbox is not ready", retryable: true}})
  end

  defp render_error(conn, :stdin_not_enabled) do
    conn
    |> put_status(:unprocessable_entity)
    |> json(%{error: %{code: "stdin_not_enabled", message: "Command stdin is not enabled", retryable: false}})
  end

  defp render_error(conn, :command_not_running) do
    conn
    |> put_status(:conflict)
    |> json(%{error: %{code: "command_not_running", message: "Command is not running", retryable: false}})
  end

  defp render_error(conn, :invalid_request) do
    conn
    |> put_status(:unprocessable_entity)
    |> json(%{error: %{code: "invalid_request", message: "Invalid request payload", retryable: false}})
  end

  defp render_error(conn, reason) do
    conn
    |> put_status(:unprocessable_entity)
    |> json(%{error: %{code: to_string(reason), message: "Request failed", retryable: false}})
  end
end
