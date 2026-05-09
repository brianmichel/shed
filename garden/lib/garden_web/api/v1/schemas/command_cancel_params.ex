defmodule GardenWeb.Api.V1.Schemas.CommandCancelParams do
  use Ecto.Schema
  import Ecto.Changeset

  @primary_key false
  embedded_schema do
    field :grace_period_ms, :integer, default: 5_000
    field :escalation, :string, default: "kill"
  end

  def changeset(schema, params) do
    schema
    |> cast(normalize(params), [:grace_period_ms, :escalation])
    |> validate_required([:grace_period_ms, :escalation])
    |> validate_number(:grace_period_ms, greater_than_or_equal_to: 0)
    |> validate_inclusion(:escalation, ["kill"])
  end

  def to_attrs(params) do
    %{"grace_period_ms" => params.grace_period_ms, "escalation" => params.escalation}
  end

  defp normalize(params) do
    params
    |> Map.put_new("grace_period_ms", 5_000)
    |> Map.put_new("escalation", "kill")
  end
end
