defmodule Garden.SeedProtocol.Payload do
  @moduledoc false

  defmacro __using__(opts) do
    {fields, _} = Code.eval_quoted(Keyword.fetch!(opts, :fields), [], __CALLER__)
    {defaults, _} = Code.eval_quoted(Keyword.get(opts, :defaults, %{}), [], __CALLER__)
    {required, _} = Code.eval_quoted(Keyword.get(opts, :required, []), [], __CALLER__)
    {inclusion, _} = Code.eval_quoted(Keyword.get(opts, :inclusion, %{}), [], __CALLER__)
    {validate_number, _} = Code.eval_quoted(Keyword.get(opts, :validate_number, []), [], __CALLER__)

    field_defs =
      Enum.map(fields, fn {field, type} ->
        default = Map.get(defaults, field)
        quote do: field(unquote(field), unquote(type), default: unquote(default))
      end)

    field_names = Enum.map(fields, &elem(&1, 0))

    quote do
      use Ecto.Schema
      import Ecto.Changeset

      @primary_key false
      embedded_schema do
        unquote_splicing(field_defs)
      end

      def changeset(schema, params) do
        schema
        |> cast(params, unquote(field_names))
        |> validate_required(unquote(required))
        |> validate_inclusions(unquote(Macro.escape(inclusion)))
        |> validate_numbers(unquote(Macro.escape(validate_number)))
      end

      def to_payload(struct) do
        struct
        |> Map.from_struct()
        |> Enum.reject(fn {_k, v} -> is_nil(v) end)
        |> Map.new(fn {k, v} -> {to_string(k), v} end)
      end

      defp validate_inclusions(changeset, inclusion) do
        Enum.reduce(inclusion, changeset, fn {field, values}, acc ->
          validate_inclusion(acc, field, values)
        end)
      end

      defp validate_numbers(changeset, validations) do
        Enum.reduce(validations, changeset, fn {field, opts}, acc ->
          validate_number(acc, field, opts)
        end)
      end
    end
  end
end
