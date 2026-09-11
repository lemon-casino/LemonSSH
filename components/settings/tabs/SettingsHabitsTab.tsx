import React from "react";
import { useI18n } from "../../../application/i18n/I18nProvider";
import type { CloseBehavior } from "../../../domain/closeBehavior";
import type { LayoutMode } from "../../../domain/layoutMode";
import { SectionHeader, SettingCard, SettingRow, SettingsTabContent, Select } from "../settings-ui";

interface SettingsHabitsTabProps {
  closeBehavior: CloseBehavior | null;
  setCloseBehavior: (behavior: CloseBehavior | null) => void;
  layoutMode: LayoutMode;
  setLayoutMode: (mode: LayoutMode) => void;
}

const SettingsHabitsTab: React.FC<SettingsHabitsTabProps> = ({
  closeBehavior,
  setCloseBehavior,
  layoutMode,
  setLayoutMode,
}) => {
  const { t } = useI18n();
  return (
    <SettingsTabContent value="habits">
      <SectionHeader title={t("settings.habits.title")} />
      <SettingCard>
        <SettingRow
          anchorId="habits-close-behavior"
          label={t("settings.habits.closeBehavior")}
          description={t("settings.habits.closeBehavior.desc")}
        >
          <Select
            value={closeBehavior ?? "ask"}
            onChange={(value) => {
              if (value === "minimize" || value === "quit") setCloseBehavior(value);
              else setCloseBehavior(null);
            }}
            options={[
              { value: "ask", label: t("settings.habits.closeBehavior.ask") },
              { value: "minimize", label: t("settings.habits.closeBehavior.minimize") },
              { value: "quit", label: t("settings.habits.closeBehavior.quit") },
            ]}
            className="w-48"
          />
        </SettingRow>
      </SettingCard>
      <SettingCard>
        <SettingRow
          anchorId="habits-layout"
          label={t("settings.habits.layout")}
          description={t("settings.habits.layout.desc")}
        >
          <Select
            value={layoutMode ?? "classic"}
            onChange={(value) => {
              if (value === "workbench") setLayoutMode("workbench");
              else setLayoutMode("classic");
            }}
            options={[
              { value: "classic", label: t("settings.habits.layout.classic") },
              { value: "workbench", label: t("settings.habits.layout.workbench") },
            ]}
            className="w-48"
          />
        </SettingRow>
      </SettingCard>
    </SettingsTabContent>
  );
};

export default React.memo(SettingsHabitsTab);
