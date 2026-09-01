import { useState } from 'react';
import { Package, Box, Building2, Tags, Factory } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { ProductsTab } from '../components/admin/ProductsTab';
import { ProductPackagesTab } from '../components/admin/ProductPackagesTab';
import { RentedProductsTab } from '../components/admin/RentedProductsTab';
import { CategoriesTab } from '../components/admin/CategoriesTab';
import { BrandsManufacturersTab } from '../components/admin/BrandsManufacturersTab';

type TabType = 'products' | 'categories' | 'brands' | 'packages' | 'rented';

export function ProductsPage() {
  const { t } = useTranslation();
  const [activeTab, setActiveTab] = useState<TabType>('products');

  const tabs = [
    { id: 'products' as TabType, label: 'Produkte', icon: Package },
	{ id: 'categories' as TabType, label: 'Kategorien', icon: Tags },
	{ id: 'brands' as TabType, label: 'Marken & Hersteller', icon: Factory },
    { id: 'packages' as TabType, label: 'Produktpakete', icon: Box },
    { id: 'rented' as TabType, label: 'Mietprodukte', icon: Building2 },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Package className="w-8 h-8 text-accent-red" />
        <div>
          <h1 className="text-3xl font-bold text-white">{t('productManagement.productsTitle')}</h1>
          <p className="text-gray-400">{t('productManagement.productsSubtitle')}</p>
        </div>
      </div>

      {/* Tabs */}
      <div className="glass-dark rounded-2xl p-2">
        <div className="flex gap-2 overflow-x-auto scrollbar-thin">
          {tabs.map(tab => {
            const Icon = tab.icon;
            return (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={`flex items-center justify-center gap-2 px-4 sm:px-6 py-3 rounded-xl font-semibold transition-all whitespace-nowrap flex-shrink-0 ${activeTab === tab.id
                    ? 'bg-accent-red text-white shadow-lg'
                    : 'text-gray-400 hover:bg-white/5 hover:text-white'
                  }`}
              >
                <Icon className="w-5 h-5" />
                <span className="hidden sm:inline">{tab.label}</span>
                <span className="sm:hidden text-xs">{tab.label.split(' ')[0]}</span>
              </button>
            );
          })}
        </div>
      </div>

      {/* Tab Content */}
      <div className="glass-dark rounded-2xl p-6">
        {activeTab === 'products' && <ProductsTab />}
		{activeTab === 'categories' && <CategoriesTab />}
		{activeTab === 'brands' && <BrandsManufacturersTab />}
        {activeTab === 'packages' && <ProductPackagesTab />}
        {activeTab === 'rented' && <RentedProductsTab />}
      </div>
    </div>
  );
}
