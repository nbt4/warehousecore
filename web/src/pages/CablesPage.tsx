import { Cable } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { CablesTab } from '../components/admin/CablesTab';

export function CablesPage() {
  const { t } = useTranslation();

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Cable className="w-8 h-8 text-accent-red" />
        <div>
          <h1 className="text-3xl font-bold text-light">{t('productManagement.cablesTitle')}</h1>
          <p className="text-text-muted">{t('productManagement.cablesSubtitle')}</p>
        </div>
      </div>

      <div className="card rounded-2xl p-6">
        <CablesTab />
      </div>
    </div>
  );
}
