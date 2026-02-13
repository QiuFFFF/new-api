import { getServerAddress } from './token';

export function generateCCSwitchLink(app, name, apiKey) {
  const serverAddress = getServerAddress();
  const base = serverAddress.replace(/\/+$/, '');
  const endpoint = app === 'claude' ? base : base + '/v1';
  const params = new URLSearchParams({
    resource: 'provider',
    app,
    name,
    endpoint,
    apiKey,
    enabled: 'true',
  });
  return `ccswitch://v1/import?${params.toString()}`;
}
