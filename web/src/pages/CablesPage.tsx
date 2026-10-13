import { useTranslation } from "react-i18next";
import { CablesTab } from "../components/admin/CablesTab";
import { WorkspaceHeader } from "../components/catalog/Workspace";
import "../components/catalog/catalog-workspace.css";
export function CablesPage() {
  const { t } = useTranslation();
  return (
    <div
      className="suite-page catalog-workspace space-y-5"
      style={{ color: "var(--text-primary)" }}
    >
      <WorkspaceHeader
        title={t("productManagement.cablesTitle")}
        description={t("productManagement.cablesSubtitle")}
      />
      <CablesTab />
    </div>
  );
}
