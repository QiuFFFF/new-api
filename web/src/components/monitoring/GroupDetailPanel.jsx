import React, { useState, useEffect, useCallback } from 'react';
import { Table, Tag, Spin, SideSheet, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError } from '../../helpers';
import AvailabilityCacheChart from './AvailabilityCacheChart';
import { useIsMobile } from '../../hooks/common/useIsMobile';

const { Text } = Typography;

const GroupDetailPanel = ({ groupName, isAdmin: isAdminUser, onClose, visible }) => {
  const { t } = useTranslation();
  const isMobile = useIsMobile();
  const [history, setHistory] = useState([]);
  const [channelStats, setChannelStats] = useState([]);
  const [periodMinutes, setPeriodMinutes] = useState(1440);
  const [intervalMinutes, setIntervalMinutes] = useState(5);
  const [loading, setLoading] = useState(true);

  const formatPeriod = (minutes) => {
    if (!minutes || minutes <= 0) minutes = 1440;
    if (minutes < 60) {
      return t('最近{{count}}分钟', { count: minutes });
    } else if (minutes < 1440) {
      const hours = Math.round(minutes / 60);
      return t('最近{{count}}小时', { count: hours });
    } else {
      const days = Math.round(minutes / 1440);
      return t('最近{{count}}天', { count: days });
    }
  };

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      // Fetch history data
      const historyEndpoint = isAdminUser
        ? `/api/monitoring/admin/groups/${encodeURIComponent(groupName)}/history`
        : `/api/monitoring/public/groups/${encodeURIComponent(groupName)}/history`;
      const historyRes = await API.get(historyEndpoint, { disableDuplicate: true });
      if (historyRes.data.success) {
        setHistory(historyRes.data.data || []);
        if (historyRes.data.period_minutes) {
          setPeriodMinutes(historyRes.data.period_minutes);
        }
        if (historyRes.data.aggregation_interval_minutes) {
          setIntervalMinutes(historyRes.data.aggregation_interval_minutes);
        }
      }

      // Fetch channel details (admin only)
      if (isAdminUser) {
        const detailRes = await API.get(
          `/api/monitoring/admin/groups/${encodeURIComponent(groupName)}`,
          { disableDuplicate: true }
        );
        if (detailRes.data.success) {
          setChannelStats(detailRes.data.channel_stats || []);
        }
      }
    } catch (error) {
      showError(t('获取详情数据失败'));
    } finally {
      setLoading(false);
    }
  }, [groupName, isAdminUser, t]);

  useEffect(() => {
    if (visible && groupName) {
      fetchData();
    }
  }, [visible, groupName, fetchData]);

  const formatRate = (rate) => {
    if (rate < 0) return '--';
    return rate.toFixed(1) + '%';
  };

  const formatTime = (timestamp) => {
    if (!timestamp) return '--';
    return new Date(timestamp * 1000).toLocaleString();
  };

  const channelColumns = [
    {
      title: t('渠道ID'),
      dataIndex: 'channel_id',
      width: 80,
    },
    {
      title: t('渠道名称'),
      dataIndex: 'channel_name',
      width: 150,
      ellipsis: true,
    },
    {
      title: t('状态'),
      dataIndex: 'is_online',
      width: 80,
      render: (val) => (
        <Tag color={val ? 'green' : 'red'} size='small'>
          {val ? t('在线') : t('离线')}
        </Tag>
      ),
    },
    {
      title: t('渠道状态'),
      dataIndex: 'channel_status',
      width: 100,
      render: (val) => {
        if (val === 2) {
          return <Tag color='yellow' size='small'>{t('手动禁用')}</Tag>;
        }
        if (val === 3) {
          return <Tag color='orange' size='small'>{t('自动禁用')}</Tag>;
        }
        return <Tag color='green' size='small'>{t('已启用')}</Tag>;
      },
    },
    {
      title: t('可用率'),
      dataIndex: 'availability_rate',
      width: 100,
      render: (val) => {
        const color = val < 0 ? 'grey' : val >= 95 ? 'green' : val >= 90 ? 'lime' : val >= 85 ? 'yellow' : val >= 75 ? 'orange' : 'red';
        return (
          <Tag color={color} size='small' type='light'>
            {formatRate(val)}
          </Tag>
        );
      },
    },
    {
      title: t('缓存率'),
      dataIndex: 'cache_hit_rate',
      width: 100,
      render: (val) => {
        const effective = (val >= 0 && val < 3) ? -1 : val;
        if (effective < 0) {
          return (
            <Tag color='grey' size='small' type='light'>
              {t('尚未获取')}
            </Tag>
          );
        }
        const color = effective >= 95 ? 'green' : effective >= 90 ? 'lime' : effective >= 85 ? 'yellow' : effective >= 75 ? 'orange' : 'red';
        return (
          <Tag color={color} size='small' type='light'>
            {formatRate(effective)}
          </Tag>
        );
      },
    },
    {
      title: t('首字速度'),
      dataIndex: 'last_frt',
      width: 100,
      render: (val) => (val > 0 ? val + 'ms' : '--'),
    },
    {
      title: t('测试模型'),
      dataIndex: 'last_test_model',
      width: 150,
      ellipsis: true,
      render: (val) => val || '--',
    },
    {
      title: t('最后测试时间'),
      dataIndex: 'last_test_time',
      width: 180,
      render: (val) => formatTime(val),
    },
  ];

  return (
    <SideSheet
      placement='right'
      visible={visible}
      onCancel={onClose}
      width={isMobile ? '100%' : 640}
      title={`${groupName || ''} - ${t('详情')}`}
      bodyStyle={{ padding: 20 }}
    >
      {loading ? (
        <div className='flex items-center justify-center py-8'>
          <Spin size='large' />
        </div>
      ) : (
        <>
          {/* Chart */}
          <div className='mb-4'>
            <Text type='secondary' className='mb-2 block'>
              {t('可用率')} & {t('缓存命中率')} ({formatPeriod(periodMinutes)})
            </Text>
            <AvailabilityCacheChart
              history={history}
              periodMinutes={periodMinutes}
              intervalMinutes={intervalMinutes}
            />
          </div>

          {/* Channel table (admin only) */}
          {isAdminUser && channelStats.length > 0 && (
            <div>
              <Text type='secondary' className='mb-2 block'>
                {t('渠道详情')}
              </Text>
              <Table
                columns={channelColumns}
                dataSource={channelStats}
                rowKey='id'
                pagination={false}
                size='small'
              />
            </div>
          )}
        </>
      )}
    </SideSheet>
  );
};

export default GroupDetailPanel;
