#!/usr/bin/env node

import process from "node:process";

import { checkPluginCompatibility } from "./compatibility.js";
import { buildPlugin, initPlugin, packPlugin, validateTarget } from "./commands.js";
import { manifestIdentity } from "./manifest.js";

const USAGE = `LemonSSH plugin CLI (API 0.1.0-internal)

Usage:
  lemonssh-plugin init <directory> --id <reverse.dns.id> [--name <display name>]
  lemonssh-plugin validate <directory|package.ncpkg>
  lemonssh-plugin compatibility <directory|package.ncpkg> --lemonssh <version> [--api <version>] [--features <id,id,...>]
  lemonssh-plugin build <directory>
  lemonssh-plugin pack <directory> [--out <package.ncpkg>]
`;

function optionValue(args: readonly string[], name: string): string | undefined {
  const index = args.indexOf(name);
  if (index === -1) return undefined;
  const value = args[index + 1];
  if (!value || value.startsWith("--")) throw new Error(`Missing value for ${name}`);
  return value;
}

async function main(args: readonly string[]): Promise<void> {
  const [command, target] = args;
  if (!command || command === "help" || command === "--help" || command === "-h") {
    process.stdout.write(USAGE);
    return;
  }
  if (!target) throw new Error(`Missing target for ${command}`);

  if (command === "init") {
    const id = optionValue(args, "--id");
    if (!id) throw new Error("init requires --id <reverse.dns.id>");
    const directory = await initPlugin(target, { id, name: optionValue(args, "--name") });
    process.stdout.write(`Initialized plugin in ${directory}\n`);
    return;
  }
  if (command === "validate") {
    const result = await validateTarget(target);
    const identity = manifestIdentity(result.manifest);
    process.stdout.write(
      `Valid ${result.kind}: ${identity.id}@${identity.version}\n`,
    );
    return;
  }
  if (command === "compatibility") {
    const lemonsshVersion = optionValue(args, "--lemonssh");
    if (!lemonsshVersion) {
      throw new Error("compatibility requires --lemonssh <version>");
    }
    const targetResult = await validateTarget(target);
    const features = optionValue(args, "--features")
      ?.split(",")
      .map((feature) => feature.trim())
      .filter(Boolean);
    const result = checkPluginCompatibility(targetResult.manifest, {
      lemonsshVersion,
      apiVersion: optionValue(args, "--api"),
      features,
    });
    if (!result.compatible) {
      throw new Error(`Plugin is incompatible:\n- ${result.errors.join("\n- ")}`);
    }
    const featureSummary = result.enabledFeatures.length > 0
      ? result.enabledFeatures.join(", ")
      : "none";
    const identity = manifestIdentity(targetResult.manifest);
    process.stdout.write(
      `Compatible: ${identity.id}@${identity.version}\nEnabled features: ${featureSummary}\n`,
    );
    return;
  }
  if (command === "build") {
    await buildPlugin(target);
    process.stdout.write("Plugin build completed.\n");
    return;
  }
  if (command === "pack") {
    const result = await packPlugin(target, optionValue(args, "--out"));
    process.stdout.write(
      `Packed ${result.fileCount} files to ${result.outputPath}\nSHA-256 ${result.sha256}\n`,
    );
    return;
  }
  throw new Error(`Unknown command: ${command}`);
}

main(process.argv.slice(2)).catch((error: unknown) => {
  process.stderr.write(`${error instanceof Error ? error.message : String(error)}\n`);
  process.exitCode = 1;
});
