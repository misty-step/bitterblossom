#!/bin/sh
# Validate configured OpenRouter API-harness agent models against a catalog
# fixture or the live OpenRouter model catalog.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

agents_dir="tests/fixtures/model-catalog-agents"
docs_path="docs/model-evals/README.md"
catalog_path=
live=false
json=false
catalog_url=${OPENROUTER_MODELS_URL:-https://openrouter.ai/api/v1/models}

usage() {
  cat <<'USAGE'
usage: scripts/check-model-catalog.sh (--catalog PATH | --live) [--agents DIR] [--docs PATH] [--json]

Validates configured pi/omp OpenRouter model fixtures against an OpenRouter
catalog document. The default docs target is docs/model-evals/README.md.

Options:
  --catalog PATH   Read catalog JSON from PATH.
  --live           Fetch https://openrouter.ai/api/v1/models.
  --agents DIR     Agent config directory. Default: tests/fixtures/model-catalog-agents.
  --docs PATH      Documentation file or directory to search for model ids.
                   Default: docs/model-evals/README.md.
  --json           Emit stable JSON instead of human text.
  -h, --help       Show this help.
USAGE
}

fail_usage() {
  echo "check-model-catalog: $*" >&2
  usage >&2
  exit 2
}

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "check-model-catalog: missing required command: $1" >&2
    exit 2
  }
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --catalog)
      [ "$#" -ge 2 ] || fail_usage "--catalog needs a path"
      catalog_path=$2
      shift 2
      ;;
    --live)
      live=true
      shift
      ;;
    --agents)
      [ "$#" -ge 2 ] || fail_usage "--agents needs a path"
      agents_dir=$2
      shift 2
      ;;
    --docs)
      [ "$#" -ge 2 ] || fail_usage "--docs needs a path"
      docs_path=$2
      shift 2
      ;;
    --json)
      json=true
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      fail_usage "unknown argument: $1"
      ;;
  esac
done

if [ "$live" = true ] && [ -n "$catalog_path" ]; then
  fail_usage "choose either --catalog or --live, not both"
fi
if [ "$live" = false ] && [ -z "$catalog_path" ]; then
  fail_usage "choose --catalog PATH or --live"
fi

need jq
need awk
need grep
if [ "$live" = true ]; then
  need curl
fi

[ -d "$agents_dir" ] || {
  echo "check-model-catalog: missing agents directory: $agents_dir" >&2
  exit 2
}

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT INT TERM

family_module="$tmpdir/model-family.jq"
cat >"$family_module" <<'JQ'
def family($id):
  if ($id | startswith("deepseek/")) then "deepseek"
  elif ($id | startswith("moonshotai/")) then "kimi"
  elif ($id | startswith("z-ai/")) then "glm"
  elif ($id | startswith("x-ai/")) then "grok"
  elif ($id | startswith("openai/")) then "openai"
  else null
  end;
JQ

catalog="$tmpdir/catalog.json"
catalog_source=$catalog_path
if [ "$live" = true ]; then
  catalog_source=$catalog_url
  if [ -n "${OPENROUTER_API_KEY:-}" ]; then
    {
      printf '%s\n' 'fail'
      printf '%s\n' 'silent'
      printf '%s\n' 'show-error'
      printf '%s\n' 'location'
      printf 'url = "%s"\n' "$catalog_url"
      printf 'header = "Authorization: Bearer %s"\n' "$OPENROUTER_API_KEY"
    } | curl --config - >"$catalog" || {
      echo "check-model-catalog: live fetch failed: $catalog_url" >&2
      exit 2
    }
  else
    curl -fsSL "$catalog_url" >"$catalog" || {
      echo "check-model-catalog: live fetch failed: $catalog_url" >&2
      exit 2
    }
  fi
else
  [ -f "$catalog_path" ] || {
    echo "check-model-catalog: missing catalog: $catalog_path" >&2
    exit 2
  }
  cp "$catalog_path" "$catalog"
fi

jq -e '.data | type == "array"' "$catalog" >/dev/null || {
  echo "check-model-catalog: catalog must be an object with a data array" >&2
  exit 1
}

configured_tsv="$tmpdir/configured.tsv"
: >"$configured_tsv"
for file in "$agents_dir"/*.toml; do
  [ -e "$file" ] || continue
  awk -v file="$file" '
    function value(line) {
      sub(/#.*/, "", line)
      sub(/^[^=]*=[[:space:]]*/, "", line)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", line)
      if (line ~ /^"/) {
        sub(/^"/, "", line)
        sub(/"$/, "", line)
      } else if (line ~ /^'\''/) {
        sub(/^'\''/, "", line)
        sub(/'\''$/, "", line)
      }
      return line
    }
    /^[[:space:]]*harness[[:space:]]*=/ { harness = value($0) }
    /^[[:space:]]*provider[[:space:]]*=/ { provider = value($0) }
    /^[[:space:]]*model[[:space:]]*=/ { model = value($0) }
    END {
      if ((harness == "pi" || harness == "omp") && model != "" && (provider == "" || provider == "openrouter")) {
        catalog_model = model
        sub(/:(minimal|low|medium|high|max)$/, "", catalog_model)
        print file "\t" model "\t" catalog_model
      }
    }
  ' "$file" >>"$configured_tsv"
done

configured_json="$tmpdir/configured.json"
jq -Rn '
  [inputs
   | select(length > 0)
   | split("\t")
   | {agent_file: .[0], id: .[1], catalog_id: .[2]}]
  | sort_by(.id, .agent_file)
' "$configured_tsv" >"$configured_json"

missing_json="$tmpdir/missing.json"
jq --slurpfile configured "$configured_json" '
  INDEX(.data[]; .id) as $catalog
  | $configured[0]
  | map(select($catalog[.catalog_id] == null))
' "$catalog" >"$missing_json"

metadata_gaps_json="$tmpdir/metadata-gaps.json"
jq --slurpfile configured "$configured_json" '
  def missing_field($m; $path):
    ($m | getpath($path)?) as $v
    | ($v == null)
      or (($v | type) == "string" and ($v | length) == 0)
      or (($path == ["context_length"] or $path == ["top_provider", "context_length"])
          and (($v | tonumber? // 0) <= 0));

  INDEX(.data[]; .id) as $catalog
  | $configured[0]
  | map(. as $c
      | $catalog[$c.catalog_id] as $m
      | if $m == null then
          empty
        else
          [
            (if missing_field($m; ["name"]) then "name" else empty end),
            (if missing_field($m; ["context_length"]) then "context_length" else empty end),
            (if missing_field($m; ["pricing", "prompt"]) then "pricing.prompt" else empty end),
            (if missing_field($m; ["pricing", "completion"]) then "pricing.completion" else empty end),
            (if missing_field($m; ["architecture", "input_modalities"]) then "architecture.input_modalities" else empty end),
            (if missing_field($m; ["architecture", "output_modalities"]) then "architecture.output_modalities" else empty end),
            (if missing_field($m; ["top_provider", "context_length"]) then "top_provider.context_length" else empty end)
          ] as $fields
          | if ($fields | length) == 0 then
              empty
            else
              {agent_file: $c.agent_file, id: $c.id, catalog_id: $c.catalog_id, fields: $fields}
            end
        end)
' "$catalog" >"$metadata_gaps_json"

configured_enriched_json="$tmpdir/configured-enriched.json"
jq --slurpfile configured "$configured_json" '
  INDEX(.data[]; .id) as $catalog
  | $configured[0]
  | map(. as $c
      | $catalog[$c.catalog_id] as $m
      | $c + {
          name: ($m.name // null),
          context_length: ($m.context_length // null),
          pricing: {
            prompt: ($m.pricing.prompt // null),
            completion: ($m.pricing.completion // null)
          },
          top_provider: ($m.top_provider // null),
          architecture: ($m.architecture // null),
          supported_parameters: ($m.supported_parameters // null)
        })
' "$catalog" >"$configured_enriched_json"

docs_missing_json="$tmpdir/docs-missing.json"
docs_missing_tsv="$tmpdir/docs-missing.tsv"
: >"$docs_missing_tsv"
if [ ! -e "$docs_path" ]; then
  while IFS='	' read -r agent_file model_id catalog_id; do
    [ -n "$model_id" ] || continue
    printf '%s\t%s\t%s\t%s\n' "$agent_file" "$model_id" "$catalog_id" "$docs_path" >>"$docs_missing_tsv"
  done <"$configured_tsv"
else
  while IFS='	' read -r agent_file model_id catalog_id; do
    [ -n "$model_id" ] || continue
    found=false
    if [ -d "$docs_path" ]; then
      grep -R -F -- "$model_id" "$docs_path" >/dev/null 2>&1 && found=true
      [ "$found" = true ] || grep -R -F -- "$catalog_id" "$docs_path" >/dev/null 2>&1 && found=true
    else
      grep -F -- "$model_id" "$docs_path" >/dev/null 2>&1 && found=true
      [ "$found" = true ] || grep -F -- "$catalog_id" "$docs_path" >/dev/null 2>&1 && found=true
    fi
    [ "$found" = true ] || printf '%s\t%s\t%s\t%s\n' "$agent_file" "$model_id" "$catalog_id" "$docs_path" >>"$docs_missing_tsv"
  done <"$configured_tsv"
fi
jq -Rn '
  [inputs
   | select(length > 0)
   | split("\t")
   | {agent_file: .[0], id: .[1], catalog_id: .[2], docs_path: .[3]}]
  | sort_by(.id, .agent_file)
  | group_by(.id)
  | map({
      id: .[0].id,
      catalog_id: .[0].catalog_id,
      docs_path: .[0].docs_path,
      agent_files: map(.agent_file)
    })
' "$docs_missing_tsv" >"$docs_missing_json"

candidates_json="$tmpdir/candidates.json"
jq -L "$tmpdir" --slurpfile configured "$configured_json" '
  include "model-family";
  ($configured[0] | map(.catalog_id)) as $configured_ids
  | [.data[]
      | . as $m
      | family($m.id) as $family
      | select($family != null)
      | select(($configured_ids | index($m.id)) | not)
      | {
          family: $family,
          id: $m.id,
          name: ($m.name // null),
          created: ($m.created // null),
          context_length: ($m.context_length // null),
          pricing: {
            prompt: ($m.pricing.prompt // null),
            completion: ($m.pricing.completion // null)
          },
          promotion_smoke: ("bb --config <runtime-plane> run <flow> --payload '\''{\"model\":\"" + $m.id + "\",\"dry_run\":true}'\'' --json")
        }]
  | sort_by(.created // 0)
  | reverse
  | .[:20]
' "$catalog" >"$candidates_json"

configured_successors_json="$tmpdir/configured-successors.json"
jq -L "$tmpdir" --slurpfile configured "$configured_json" '
  include "model-family";
  (.data) as $models
  | INDEX($models[]; .id) as $catalog
  | $configured[0]
  | map(. as $c
      | $catalog[$c.catalog_id] as $current
      | family($c.catalog_id) as $family
      | if $current == null or $family == null then
          empty
        else
          [$models[]
            | . as $candidate
            | select(family($candidate.id) == $family)
            | select($candidate.id != $c.catalog_id)
            | select((($candidate.created // 0) | tonumber) > (($current.created // 0) | tonumber))
            | {
                id: $candidate.id,
                name: ($candidate.name // null),
                created: ($candidate.created // null),
                context_length: ($candidate.context_length // null),
                alias: ($candidate.id | startswith("~")),
                pricing: {
                  prompt: ($candidate.pricing.prompt // null),
                  completion: ($candidate.pricing.completion // null)
                },
                promotion_smoke: ("bb --config <runtime-plane> run <flow> --payload '\''{\"model\":\"" + $candidate.id + "\",\"dry_run\":true}'\'' --json")
              }]
          | sort_by(.created // 0)
          | reverse
          | if length == 0 then
              empty
            else
              {
                agent_file: $c.agent_file,
                current: {
                  id: $c.id,
                  catalog_id: $c.catalog_id,
                  name: ($current.name // null),
                  created: ($current.created // null)
                },
                family: $family,
                successors: .[:5]
              }
            end
        end)
' "$catalog" >"$configured_successors_json"

if jq -e 'length == 0' "$missing_json" >/dev/null &&
   jq -e 'length == 0' "$metadata_gaps_json" >/dev/null &&
   jq -e 'length == 0' "$docs_missing_json" >/dev/null; then
  status=pass
else
  status=fail
fi

report_json="$tmpdir/report.json"
jq -n \
  --arg status "$status" \
  --arg provider "openrouter" \
  --arg catalog_source "$catalog_source" \
  --arg docs_path "$docs_path" \
  --slurpfile configured "$configured_enriched_json" \
  --slurpfile missing "$missing_json" \
  --slurpfile metadata_gaps "$metadata_gaps_json" \
  --slurpfile docs_missing "$docs_missing_json" \
  --slurpfile candidates "$candidates_json" \
  --slurpfile configured_successors "$configured_successors_json" \
  '{
    status: $status,
    provider: $provider,
    catalog_source: $catalog_source,
    docs_path: $docs_path,
    configured: $configured[0],
    missing: $missing[0],
    metadata_gaps: $metadata_gaps[0],
    docs_missing: $docs_missing[0],
    new_family_candidates: $candidates[0],
    configured_successors: $configured_successors[0],
    promotion_required_evidence: [
      "successful bb smoke run for the affected flow",
      "model-eval reference record under docs/model-evals/",
      "green ./scripts/verify.sh",
      "explicit reviewed PR changing runtime agent config; no automatic promotion"
    ]
  }' >"$report_json"

if [ "$json" = true ]; then
  cat "$report_json"
else
  jq -r '
    "status: \(.status)",
    "provider: \(.provider)",
    "catalog: \(.catalog_source)",
    "configured: \(.configured | length)",
    "missing: \(.missing | length)",
    "metadata_gaps: \(.metadata_gaps | length)",
    "docs_missing: \(.docs_missing | length)",
    "new_family_candidates: \(.new_family_candidates | length)",
    "configured_successors: \(.configured_successors | length)"
  ' "$report_json"
  jq -r '.missing[]? | "missing model: \(.id) in \(.agent_file)"' "$report_json"
  jq -r '.metadata_gaps[]? | "metadata gap: \(.id) -> \(.fields | join(", "))"' "$report_json"
  jq -r '.docs_missing[]? | "docs missing: \(.id) in \(.docs_path)"' "$report_json"
  jq -r '.configured_successors[]? | "newer configured-family model: \(.current.id) in \(.agent_file) -> \([.successors[].id] | join(", "))"' "$report_json"
fi

[ "$status" = pass ] || exit 1
