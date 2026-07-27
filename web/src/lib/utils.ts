import { type ClassValue, clsx } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function getStatusColor(status: string): string {
  switch (status.toLowerCase()) {
    case 'in_storage':
      return 'text-gray-400';
    case 'on_job':
    case 'rented':
      return 'text-accent-red';
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
  return status.replace('_', ' ').replace(/\b\w/g, l => l.toUpperCase());
}
