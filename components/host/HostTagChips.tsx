import React from "react";

import { cn } from "../../lib/utils";

export const VISIBLE_HOST_TAG_LIMIT = 2;

export function visibleHostTags(
  tags: readonly string[] | undefined,
  limit = VISIBLE_HOST_TAG_LIMIT,
): { shown: string[]; extra: number } {
  const cleaned = (tags ?? [])
    .map((tag) => tag.trim())
    .filter((tag) => tag.length > 0);
  return {
    shown: cleaned.slice(0, limit),
    extra: Math.max(0, cleaned.length - limit),
  };
}

export function toggleSelectedTag(selected: readonly string[], tag: string): string[] {
  return selected.includes(tag)
    ? selected.filter((item) => item !== tag)
    : [...selected, tag];
}

export interface HostTagChipsProps {
  tags?: readonly string[];
  selectedTags?: readonly string[];
  onToggleTag?: (tag: string) => void;
  compact?: boolean;
  className?: string;
}

export const HostTagChips: React.FC<HostTagChipsProps> = ({
  tags,
  selectedTags,
  onToggleTag,
  compact = false,
  className,
}) => {
  const { shown, extra } = visibleHostTags(tags);
  if (shown.length === 0) return null;

  const selectedSet = selectedTags && selectedTags.length > 0
    ? new Set(selectedTags)
    : null;

  return (
    <span
      data-host-tags=""
      className={cn("flex min-w-0 shrink-0 items-center gap-1", className)}
    >
      {shown.map((tag) => {
        const isSelected = selectedSet?.has(tag) === true;
        const chipClass = cn(
          "truncate rounded bg-primary/10 px-1.5 py-0.5 text-[10px] leading-none text-primary",
          compact && "max-w-[72px]",
          isSelected && "bg-primary/20 ring-1 ring-primary/30",
        );
        if (!onToggleTag) {
          return (
            <span key={tag} data-host-tag={tag} className={chipClass} title={tag}>
              {tag}
            </span>
          );
        }
        return (
          <button
            key={tag}
            type="button"
            data-host-tag={tag}
            title={tag}
            className={cn(chipClass, "cursor-pointer border-0")}
            onClick={(event) => {
              event.preventDefault();
              event.stopPropagation();
              onToggleTag(tag);
            }}
            onPointerDown={(event) => event.stopPropagation()}
            onMouseDown={(event) => event.stopPropagation()}
          >
            {tag}
          </button>
        );
      })}
      {extra > 0 && (
        <span className="shrink-0 text-[10px] leading-none text-muted-foreground" data-host-tag-extra="">
          +{extra}
        </span>
      )}
    </span>
  );
};
