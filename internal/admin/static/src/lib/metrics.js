export function parseMetrics(metricsText) {
  const samples = metricsText
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line !== "" && !line.startsWith("#"))
    .map(parseSample)
    .filter(Boolean);

  return {
    up: lastValueBySuffix(samples, "_up"),
    ready: lastValueBySuffix(samples, "_ready"),
    requests: sumValuesBySuffix(samples, "_requests_total"),
    fallbacks: sumValuesBySuffix(samples, "_fallbacks_total"),
    upstreamErrors: sumValuesBySuffix(samples, "_upstream_errors_total"),
    tokenUsage: sumValuesBySuffix(samples, "_token_usage_total"),
    budgetRejected: sumValuesBySuffix(samples, "_token_budget_rejected_total"),
    successRates: successRates(samples),
  };
}

function parseSample(line) {
  const match = line.match(/^([a-zA-Z_:][a-zA-Z0-9_:]*)(?:\{([^}]*)\})?\s+([-+]?[\d.]+(?:e[-+]?\d+)?)$/i);
  if (!match) {
    return null;
  }

  return {
    name: match[1],
    labels: parseLabels(match[2] ?? ""),
    value: Number(match[3]),
  };
}

function parseLabels(rawLabels) {
  if (rawLabels === "") {
    return {};
  }

  return rawLabels.split(",").reduce((labels, pair) => {
    const [key, rawValue] = pair.split("=");
    if (!key || rawValue === undefined) {
      return labels;
    }

    labels[key.trim()] = rawValue.trim().replace(/^"|"$/g, "");
    return labels;
  }, {});
}

function sumValuesBySuffix(samples, suffix) {
  return samples
    .filter((sample) => sample.name.endsWith(suffix))
    .reduce((sum, sample) => sum + safeNumber(sample.value), 0);
}

function lastValueBySuffix(samples, suffix) {
  const sample = [...samples].reverse().find((item) => item.name.endsWith(suffix));
  return sample ? safeNumber(sample.value) : null;
}

function successRates(samples) {
  return samples
    .filter((sample) => sample.name.endsWith("_request_success_rate"))
    .map((sample) => ({
      backend: sample.labels.backend ?? "unknown",
      value: safeNumber(sample.value),
    }))
    .sort((left, right) => left.backend.localeCompare(right.backend, "zh-CN"));
}

function safeNumber(value) {
  return Number.isFinite(value) ? value : 0;
}
