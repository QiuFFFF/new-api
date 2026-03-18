import React, { useEffect, useMemo } from 'react';
import { VChart } from '@visactor/react-vchart';
import { initVChartSemiTheme } from '@visactor/vchart-semi-theme';
import { useTranslation } from 'react-i18next';

const AvailabilityCacheChart = ({ history, periodMinutes, intervalMinutes }) => {
  useEffect(() => {
    initVChartSemiTheme({
      isWatchingThemeSwitch: true,
    });
  }, []);
  const { t } = useTranslation();

  const chartData = useMemo(() => {
    if (!history || history.length === 0) return [];

    const stepSec = (intervalMinutes || 5) * 60;
    const periodSec = (periodMinutes || 1440) * 60;
    const now = Math.floor(Date.now() / 1000);

    // Align a timestamp to the nearest step boundary
    const align = (ts) => Math.round(ts / stepSec) * stepSec;

    // Build map: aligned timestamp -> latest item
    const dataMap = new Map();
    history.forEach((item) => {
      const key = align(item.recorded_at);
      const existing = dataMap.get(key);
      if (!existing || item.recorded_at > existing.recorded_at) {
        dataMap.set(key, item);
      }
    });

    // Generate complete time grid
    const gridStart = align(now - periodSec);
    const gridEnd = align(now);
    const slots = [];
    for (let ts = gridStart; ts <= gridEnd; ts += stepSec) {
      slots.push(ts);
    }

    // Walk through slots — no carry-forward, show gaps for no-traffic intervals
    const result = [];

    for (const ts of slots) {
      const item = dataMap.get(ts);

      const timeStr = new Date(ts * 1000).toLocaleTimeString([], {
        hour: '2-digit',
        minute: '2-digit',
      });

      if (item && item.availability_rate >= 0) {
        result.push({
          time: timeStr,
          value: Math.min(parseFloat(item.availability_rate.toFixed(2)), 100),
          type: t('可用率'),
        });
      } else {
        result.push({
          time: timeStr,
          value: null,
          type: t('可用率'),
        });
      }
      if (item && item.cache_hit_rate >= 0) {
        result.push({
          time: timeStr,
          value: Math.min(parseFloat(item.cache_hit_rate.toFixed(2)), 100),
          type: t('缓存命中率'),
        });
      } else {
        result.push({
          time: timeStr,
          value: null,
          type: t('缓存命中率'),
        });
      }
    }

    return result;
  }, [history, periodMinutes, intervalMinutes, t]);

  if (!history || history.length === 0 || chartData.length === 0 || chartData.every(d => d.value === null)) {
    return (
      <div className='h-64 flex items-center justify-center text-gray-400'>
        {t('暂无历史数据')}
      </div>
    );
  }

  // Compute dynamic Y-axis min based on data
  const values = chartData.map(d => d.value).filter(v => v !== null);
  const minValue = values.length > 0 ? Math.min(...values) : 0;
  const yMin = Math.max(0, Math.floor(minValue) - 20);

  const spec = {
    type: 'line',
    invalidType: 'link',
    data: [
      {
        id: 'data',
        values: chartData,
      },
    ],
    xField: 'time',
    yField: 'value',
    seriesField: 'type',
    point: {
      visible: false,
    },
    line: {
      style: {
        curveType: 'monotone',
        lineWidth: 2,
      },
    },
    axes: [
      {
        orient: 'bottom',
        type: 'band',
        label: {
          autoRotate: true,
          style: {
            fontSize: 10,
          },
        },
      },
      {
        orient: 'left',
        type: 'linear',
        min: yMin,
        max: 100,
        title: {
          visible: true,
          text: '%',
        },
      },
    ],
    legends: {
      visible: true,
      orient: 'top',
    },
    tooltip: {
      visible: true,
      mark: {
        content: [
          {
            key: (datum) => datum.type,
            value: (datum) => datum.value !== null ? datum.value + '%' : '--',
          },
        ],
      },
    },
    color: ['#3b82f6', '#22c55e'],
  };

  return (
    <div className='h-64'>
      <VChart spec={spec} option={{ mode: 'desktop-browser' }} />
    </div>
  );
};

export default AvailabilityCacheChart;
