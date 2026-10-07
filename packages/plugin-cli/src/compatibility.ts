import { isManifestV2, type ValidatedPluginManifest } from "./manifest.js";
import { satisfies, valid, validRange } from "semver";

const DEFAULT_PLUGIN_API_VERSION = "0.1.0-internal";

export interface PluginCompatibilityTarget {
  readonly lemonsshVersion: string;
  readonly apiVersion?: string;
  readonly features?: readonly string[];
}

export interface PluginCompatibilityResult {
  readonly compatible: boolean;
  readonly apiVersion: string;
  readonly enabledFeatures: readonly string[];
  readonly missingRequiredFeatures: readonly string[];
  readonly errors: readonly string[];
}

function checkEngineVersion(
  label: string,
  version: string,
  range: string,
  errors: string[],
): void {
  if (valid(version) === null) {
    errors.push(`Host ${label} version is not valid semver: ${version}`);
    return;
  }
  const normalizedRange = validRange(range);
  if (normalizedRange === null) {
    errors.push(`Plugin ${label} range is not valid semver: ${range}`);
    return;
  }
  if (!satisfies(version, normalizedRange)) {
    errors.push(`Host ${label} version ${version} does not satisfy ${range}`);
  }
}

export function checkPluginCompatibility(
  manifest: ValidatedPluginManifest,
  target: PluginCompatibilityTarget,
): PluginCompatibilityResult {
  const apiVersion = target.apiVersion ?? DEFAULT_PLUGIN_API_VERSION;
  const errors: string[] = [];
  // v2 manifests (Go host) declare no engine ranges or feature gates; the only
  // host-compat field is the optional minHostVersion floor.
  if (isManifestV2(manifest)) {
    if (manifest.minHostVersion !== undefined) {
      checkEngineVersion("LemonSSH", target.lemonsshVersion, `>=${manifest.minHostVersion}`, errors);
    } else {
      const normalizedHost = valid(target.lemonsshVersion);
      if (normalizedHost === null) {
        errors.push(`Host LemonSSH version is not valid semver: ${target.lemonsshVersion}`);
      }
    }
    return {
      compatible: errors.length === 0,
      apiVersion: "2",
      enabledFeatures: [],
      missingRequiredFeatures: [],
      errors,
    };
  }
  // Schema validation already guarantees one of the two engine keys; the
  // legacy "netcatty" key keeps pre-rename manifests compatible.
  const lemonsshEngineRange = manifest.engines.lemonssh ?? manifest.engines.netcatty ?? "*";
  checkEngineVersion("LemonSSH", target.lemonsshVersion, lemonsshEngineRange, errors);
  checkEngineVersion("plugin API", apiVersion, manifest.engines.api, errors);

  const supportedFeatures = new Set(target.features ?? []);
  const requiredFeatures = manifest.features?.required ?? [];
  const optionalFeatures = manifest.features?.optional ?? [];
  const missingRequiredFeatures = requiredFeatures
    .filter((feature) => !supportedFeatures.has(feature))
    .sort((left, right) => left.localeCompare(right, "en"));
  if (missingRequiredFeatures.length > 0) {
    errors.push(`Missing required features: ${missingRequiredFeatures.join(", ")}`);
  }

  const enabledFeatures = [...new Set([...requiredFeatures, ...optionalFeatures])]
    .filter((feature) => supportedFeatures.has(feature))
    .sort((left, right) => left.localeCompare(right, "en"));

  return {
    compatible: errors.length === 0,
    apiVersion,
    enabledFeatures,
    missingRequiredFeatures,
    errors,
  };
}
