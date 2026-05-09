defmodule GardenWeb.Api.V1.SandboxController do
  use GardenWeb, :controller

  alias Garden.Sandboxes
  alias GardenWeb.Api.V1.EventStream
  alias GardenWeb.Api.V1.Schemas.{SandboxCreateParams, SandboxLeaseParams, SandboxReleaseParams}
  alias GardenWeb.Api.V1.Validation

  def index(conn, _params) do
    json(conn, %{data: Sandboxes.list_sandboxes()})
  end

  def create(conn, params) do
    with {:ok, attrs} <- Validation.validate(SandboxCreateParams, params),
         {:ok, %{sandbox: sandbox, operation: operation}} <- Sandboxes.acquire_sandbox(attrs) do
      conn
      |> put_status(:created)
      |> json(%{data: sandbox, operation: operation})
    else
      {:error, reason} -> render_error(conn, reason)
    end
  end

  def show(conn, params) do
    sandbox_id = sandbox_id!(params)

    case Sandboxes.get_sandbox(sandbox_id) do
      {:ok, sandbox} -> json(conn, %{data: sandbox})
      {:error, reason} -> render_error(conn, reason)
    end
  end

  def release(conn, params) do
    sandbox_id = sandbox_id!(params)

    with {:ok, attrs} <- Validation.validate(SandboxReleaseParams, params),
         {:ok, sandbox} <- Sandboxes.release_sandbox(sandbox_id, attrs) do
      json(conn, %{data: sandbox})
    else
      {:error, reason} -> render_error(conn, reason)
    end
  end

  def lease(conn, params) do
    sandbox_id = sandbox_id!(params)

    with {:ok, attrs} <- Validation.validate(SandboxLeaseParams, params),
         {:ok, lease} <- Sandboxes.extend_lease(sandbox_id, attrs) do
      json(conn, %{data: Map.put(lease, "sandbox_id", sandbox_id)})
    else
      {:error, reason} -> render_error(conn, reason)
    end
  end

  def events(conn, params) do
    sandbox_id = sandbox_id!(params)

    case Sandboxes.list_sandbox_events(sandbox_id, params) do
      {:ok, %{events: events, next_cursor: next_cursor}} ->
        if sse?(conn) do
          EventStream.stream(conn, events, Sandboxes.sandbox_topic(sandbox_id))
        else
          json(conn, %{data: events, next_cursor: next_cursor})
        end

      {:error, reason} ->
        render_error(conn, reason)
    end
  end

  defp sandbox_id!(params), do: Map.get(params, "sandbox_id") || Map.fetch!(params, "sandbox_sandbox_id")

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

  defp render_error(conn, reason) do
    conn
    |> put_status(:unprocessable_entity)
    |> json(%{error: %{code: to_string(reason), message: "Request failed", retryable: false}})
  end
end
