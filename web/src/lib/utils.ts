import { type ClassValue, clsx } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function getStatusColor(status: string): string {
  switch (status.toLowerCase()) {
    case 'in_storage':
      return 'text-green-500';
    case 'on_job':
    case 'rented':
      return 'text-accent-red';
    case 'return_pending':
      return 'text-yellow-400';
    case 'location_unknown':
      return 'text-orange-400';
    case 'defective':
      return 'text-yellow-500';
    case 'repair':
      return 'text-accent-red';
    case 'free':
      return 'text-green-500';
    default:
      return 'text-gray-500';
  }
}

export function formatStatus(status: string): string {
  const labels: Record<string, string> = {
    free: 'Frei',
    in_storage: 'Im Lager',
    on_job: 'Ausgegeben',
    rented: 'Vermietet',
    return_pending: 'Rückgabe offen',
    location_unknown: 'Standort ungeklärt',
    defective: 'Defekt',
    repair: 'In Reparatur',
    maintenance: 'In Wartung',
    blocked: 'Gesperrt',
    retired: 'Ausgemustert',
  };
  return labels[status.toLowerCase()] || status.replaceAll('_', ' ').replace(/\b\w/g, l => l.toUpperCase());
}
