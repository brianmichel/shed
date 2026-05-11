defmodule Garden.SeedProtocol.Payloads do
  @moduledoc false

  import Ecto.Changeset

  alias Garden.SeedProtocol.Registry

  def validate_payload(type, params) when is_map(params) do
    case Registry.schema_for(type) do
      nil -> {:ok, params}
      schema -> validate_schemaless(schema, params)
    end
  end

  def validate_payload(_type, _params) do
    {:error, cast({%{}, %{}}, %{}, []) |> add_error(:data, "must be an object")}
  end

  defp validate_schemaless(schema, params) do
    types = Map.new(schema.fields)
    fields = Keyword.keys(schema.fields)

    changeset =
      {%{}, types}
      |> cast(params, fields)
      |> validate_required(Map.get(schema, :required, []))
      |> apply_inclusions(Map.get(schema, :inclusion, %{}))

    if changeset.valid? do
      {:ok,
       changeset
       |> apply_changes()
       |> Enum.reject(fn {_k, v} -> is_nil(v) end)
       |> Map.new(fn {k, v} -> {to_string(k), v} end)}
    else
      {:error, changeset}
    end
  end

  defp apply_inclusions(changeset, inclusion) do
    Enum.reduce(inclusion, changeset, fn {field, values}, acc ->
      validate_inclusion(acc, field, values)
    end)
  end
end
