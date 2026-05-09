defmodule GardenWeb.Api.V1.FileController do
  use GardenWeb, :controller

  alias Garden.Sandboxes
  alias GardenWeb.Api.V1.Schemas.FileWriteParams
  alias GardenWeb.Api.V1.Validation

  def index(conn, params) do
    sandbox_id = sandbox_id!(params)
    path = Map.get(params, "path", "/workspace")

    case Sandboxes.list_files(sandbox_id, path) do
      {:ok, result} -> json(conn, %{data: result})
      {:error, reason} -> render_error(conn, reason)
    end
  end

  def show(conn, params) do
    sandbox_id = sandbox_id!(params)

    case Sandboxes.read_file(sandbox_id, Map.get(params, "path", "")) do
      {:ok, result} -> json(conn, %{data: result})
      {:error, reason} -> render_error(conn, reason)
    end
  end

  def update(conn, params) do
    sandbox_id = sandbox_id!(params)

    with {:ok, attrs} <- Validation.validate(FileWriteParams, params),
         {:ok, result} <- Sandboxes.write_file(sandbox_id, attrs["path"], attrs["content"]) do
      json(conn, %{data: result})
    else
      {:error, reason} -> render_error(conn, reason)
    end
  end

  defp sandbox_id!(params), do: Map.get(params, "sandbox_id") || Map.fetch!(params, "sandbox_sandbox_id")

  defp render_error(conn, %{code: _} = error), do: conn |> put_status(:unprocessable_entity) |> json(%{error: error})

  defp render_error(conn, :sandbox_not_found), do: conn |> put_status(:not_found) |> json(%{error: %{code: "sandbox_not_found", message: "Sandbox not found", retryable: false}})
  defp render_error(conn, :file_not_found), do: conn |> put_status(:not_found) |> json(%{error: %{code: "file_not_found", message: "File not found", retryable: false}})
  defp render_error(conn, reason), do: conn |> put_status(:unprocessable_entity) |> json(%{error: %{code: to_string(reason), message: "Request failed", retryable: false}})
end
