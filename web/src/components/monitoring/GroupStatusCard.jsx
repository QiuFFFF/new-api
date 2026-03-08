import React from 'react';
import { useTranslation } from 'react-i18next';
import MiniHistoryChart from './MiniHistoryChart';

const GroupStatusCard = ({ group, isSelected, onClick, history, periodMinutes, intervalMinutes }) => {
  const { t } = useTranslation();

  const isOnline = group.is_online ?? group.online_channels > 0;
  const availRate = group.availability_rate;
  const rawCacheRate = group.cache_hit_rate;
  // Cache hit rate below 3% is treated as "not yet collected"
  const cacheRate = (rawCacheRate >= 0 && rawCacheRate < 3) ? -1 : rawCacheRate;

  const formatRate = (rate) => {
    if (rate == null || isNaN(rate) || rate < 0) return '--';
    return rate.toFixed(1) + '%';
  };

  const formatCacheRate = (rate) => {
    if (rate == null || isNaN(rate) || rate < 0) return t('尚未获取');
    return rate.toFixed(1) + '%';
  };

  const formatRatio = (ratio) => {
    if (!ratio || ratio <= 0) return '--';
    return ratio.toFixed(2) + '元/刀';
  };

  const formatFrt = (frt) => {
    if (!frt || frt <= 0) return '--';
    return (frt / 1000).toFixed(2) + 's';
  };

  // Rate color mapping (unified for both availability and cache hit rate)
  const getRateColor = (rate) => {
    if (rate < 0) return 'var(--semi-color-text-2)';
    if (rate >= 95) return '#22c55e';
    if (rate >= 90) return '#65a30d';
    if (rate >= 85) return '#f59e0b';
    if (rate >= 75) return '#ea580c';
    return '#ef4444';
  };

  const getRateBg = (rate) => {
    if (rate < 0) return 'var(--semi-color-fill-0)';
    if (rate >= 95) return '#22c55e';
    if (rate >= 90) return '#65a30d';
    if (rate >= 85) return '#f59e0b';
    if (rate >= 75) return '#ea580c';
    return '#ef4444';
  };

  return (
    <div
      className={`cursor-pointer transition-all ${isSelected ? 'ring-2 ring-blue-500' : ''}`}
      onClick={onClick}
      style={{
        background: 'var(--semi-color-bg-0)',
        border: '1px solid var(--semi-color-border)',
        borderRadius: 12,
        padding: 20,
        boxShadow: '0 2px 8px rgba(0,0,0,0.08), 0 1px 3px rgba(0,0,0,0.06)',
        transition: 'all 0.25s ease',
      }}
      onMouseEnter={(e) => {
        e.currentTarget.style.boxShadow = '0 8px 24px rgba(0,0,0,0.12), 0 2px 6px rgba(0,0,0,0.06)';
        e.currentTarget.style.transform = 'translateY(-1px)';
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.boxShadow = '0 2px 8px rgba(0,0,0,0.08), 0 1px 3px rgba(0,0,0,0.06)';
        e.currentTarget.style.transform = 'translateY(0)';
      }}
    >
      {/* Top row: group name + status badge */}
      <div
        className='flex items-center'
        style={{
          marginBottom: 4,
          gap: 8,
        }}
      >
        <div
          style={{
            flex: 1,
            fontSize: 15,
            fontWeight: 650,
            color: 'var(--semi-color-text-0)',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            minWidth: 0,
          }}
          title={group.group_name}
        >
          {group.group_name}
        </div>
        <span
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: 5,
            fontSize: 11,
            fontWeight: 600,
            padding: '3px 10px',
            borderRadius: 20,
            flexShrink: 0,
            background: isOnline
              ? 'var(--semi-color-success-light-default)'
              : 'var(--semi-color-danger-light-default)',
            color: isOnline
              ? 'var(--semi-color-success)'
              : 'var(--semi-color-danger)',
          }}
        >
          <span
            style={{
              width: 6,
              height: 6,
              borderRadius: '50%',
              background: isOnline ? 'var(--semi-color-success)' : 'var(--semi-color-danger)',
            }}
          />
          {isOnline ? t('正常') : t('异常')}
        </span>
      </div>

      {/* Second row: model | FRT | price */}
      <div
        className='flex items-center'
        style={{
          marginBottom: 14,
          gap: 8,
        }}
      >
        <span
          style={{
            fontSize: 12,
            color: 'var(--semi-color-text-2)',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            minWidth: 0,
            flex: 1,
          }}
          title={group.last_test_model || '--'}
        >
          {group.last_test_model || '--'}
        </span>
        <div
          style={{
            width: 1,
            height: 12,
            background: 'var(--semi-color-border)',
            flexShrink: 0,
          }}
        />
        <span
          style={{
            fontSize: 12,
            fontWeight: 600,
            color: 'var(--semi-color-text-2)',
            fontVariantNumeric: 'tabular-nums',
            flexShrink: 0,
          }}
        >
          {formatFrt(group.avg_frt)}
        </span>
        <div
          style={{
            width: 1,
            height: 12,
            background: 'var(--semi-color-border)',
            flexShrink: 0,
          }}
        />
        <span
          style={{
            fontSize: 12,
            fontWeight: 600,
            color: 'var(--semi-color-text-2)',
            fontVariantNumeric: 'tabular-nums',
            flexShrink: 0,
          }}
        >
          {formatRatio(group.group_ratio)}
        </span>
      </div>

      {/* Availability progress bar */}
      <div style={{ marginBottom: 12 }}>
        <div className='flex items-center justify-between' style={{ marginBottom: 5 }}>
          <span style={{ fontSize: 12, color: 'var(--semi-color-text-1)', fontWeight: 500 }}>
            {t('可用率')}
          </span>
          <span
            style={{
              fontSize: 13,
              fontWeight: 650,
              fontVariantNumeric: 'tabular-nums',
              color: getRateColor(availRate),
            }}
          >
            {formatRate(availRate)}
          </span>
        </div>
        <div
          style={{
            height: 6,
            background: 'var(--semi-color-fill-0)',
            borderRadius: 3,
            overflow: 'hidden',
          }}
        >
          <div
            style={{
              height: '100%',
              borderRadius: 3,
              width: availRate >= 0 ? `${availRate}%` : '0%',
              background: getRateBg(availRate),
              transition: 'width 0.6s ease',
            }}
          />
        </div>
      </div>

      {/* Cache hit rate progress bar */}
      <div style={{ marginBottom: 0 }}>
        <div className='flex items-center justify-between' style={{ marginBottom: 5 }}>
          <span style={{ fontSize: 12, color: 'var(--semi-color-text-1)', fontWeight: 500 }}>
            {t('缓存命中率')}
          </span>
          <span
            style={{
              fontSize: 13,
              fontWeight: 650,
              fontVariantNumeric: 'tabular-nums',
              color: getRateColor(cacheRate),
            }}
          >
            {formatCacheRate(cacheRate)}
          </span>
        </div>
        <div
          style={{
            height: 6,
            background: 'var(--semi-color-fill-0)',
            borderRadius: 3,
            overflow: 'hidden',
          }}
        >
          <div
            style={{
              height: '100%',
              borderRadius: 3,
              width: cacheRate >= 0 ? `${cacheRate}%` : '0%',
              background: getRateBg(cacheRate),
              transition: 'width 0.6s ease',
            }}
          />
        </div>
      </div>

      {/* Mini chart section */}
      <div
        style={{
          marginTop: 14,
          paddingTop: 12,
          borderTop: '1px solid var(--semi-color-border)',
        }}
      >
        <div className='flex items-center justify-between' style={{ marginBottom: 8 }}>
          <span
            style={{
              fontSize: 11,
              fontWeight: 600,
              color: 'var(--semi-color-text-2)',
              textTransform: 'uppercase',
              letterSpacing: '0.05em',
            }}
          >
            HISTORY
          </span>
          <div className='flex items-center' style={{ gap: 10 }}>
            <span className='flex items-center' style={{ gap: 4, fontSize: 10, color: 'var(--semi-color-text-2)' }}>
              <span style={{ width: 8, height: 3, borderRadius: 2, background: '#3b82f6', display: 'inline-block' }} />
              {t('可用率')}
            </span>
            <span className='flex items-center' style={{ gap: 4, fontSize: 10, color: 'var(--semi-color-text-2)' }}>
              <span style={{ width: 8, height: 3, borderRadius: 2, background: '#22c55e', display: 'inline-block' }} />
              {t('缓存率')}
            </span>
          </div>
        </div>
        <MiniHistoryChart history={history} periodMinutes={periodMinutes} intervalMinutes={intervalMinutes} />
      </div>
    </div>
  );
};

export default GroupStatusCard;
