defmodule GardenWeb.Api.V1.Validation do
  @moduledoc false

  import Ecto.Changeset

  def validate(module, params) do
    changeset = module.changeset(struct(module), params)

    if changeset.valid? do
      {:ok, module.to_attrs(apply_changes(changeset))}
    else
      {:error, invalid_request(changeset)}
    end
  end

  def invalid_request(changeset) do
    %{
      code: "invalid_request",
      message: "Invalid request payload",
      retryable: false,
      details: traverse_errors(changeset, &translate_error/1)
    }
  end

  defp translate_error({message, opts}) do
    Regex.replace(~r/%{(\w+)}/, message, fn _, key ->
      opts |> Keyword.get(String.to_existing_atom(key), key) |> to_string()
    end)
  end
end
