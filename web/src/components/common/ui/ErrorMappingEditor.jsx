import React, { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Form,
  Typography,
  Banner,
  Tabs,
  TabPane,
  Card,
  Input,
  Select,
  TextArea,
  Row,
  Col,
  Divider,
} from '@douyinfe/semi-ui';
import { IconPlus, IconDelete } from '@douyinfe/semi-icons';

const { Text } = Typography;

const generateUniqueId = (() => {
  let counter = 0;
  return () => `em_${counter++}`;
})();

const MATCH_TYPE_OPTIONS = [
  { value: 'contains', labelKey: '包含匹配' },
  { value: 'regex', labelKey: '正则匹配' },
  { value: 'exact_code', labelKey: '精确代码' },
  { value: 'exact_type', labelKey: '精确类型' },
];

const TEMPLATE = {
  patterns: [
    {
      match: 'rate limit',
      match_type: 'contains',
      replace_message: '请求频率过高，请稍后重试',
    },
    {
      match: 'quota.*exceeded',
      match_type: 'regex',
      replace_message: '配额已用尽',
    },
    {
      match: 'invalid_api_key',
      match_type: 'exact_code',
      replace_message: 'API 密钥无效',
    },
  ],
};

const ErrorMappingEditor = ({
  value = '',
  onChange,
  field,
  label,
  formApi = null,
}) => {
  const { t } = useTranslation();

  const parsePatterns = useCallback((val) => {
    if (!val || typeof val !== 'string' || !val.trim()) return [];
    try {
      const parsed = JSON.parse(val);
      if (parsed && Array.isArray(parsed.patterns)) {
        return parsed.patterns.map((p) => ({
          id: generateUniqueId(),
          match: p.match || '',
          match_type: p.match_type || 'contains',
          replace_message: p.replace_message || '',
        }));
      }
    } catch {
      // ignore
    }
    return [];
  }, []);

  const [patterns, setPatterns] = useState(() => parsePatterns(value));

  const [manualText, setManualText] = useState(() => {
    if (typeof value === 'string') return value;
    return '';
  });

  const [editMode, setEditMode] = useState(() => {
    if (typeof value === 'string' && value.trim()) {
      try {
        const parsed = JSON.parse(value);
        if (parsed && Array.isArray(parsed.patterns) && parsed.patterns.length > 10) {
          return 'manual';
        }
        // Check if patterns have advanced fields (replace_type / replace_code)
        if (parsed && Array.isArray(parsed.patterns)) {
          const hasAdvanced = parsed.patterns.some(
            (p) => p.replace_type || p.replace_code,
          );
          if (hasAdvanced) return 'manual';
        }
      } catch {
        return 'manual';
      }
    }
    return 'visual';
  });

  const [jsonError, setJsonError] = useState('');

  // Sync from external value changes
  useEffect(() => {
    const newPatterns = parsePatterns(value);
    // Compare serialized to avoid unnecessary updates
    const currentJson = patternsToJson(patterns);
    if (typeof value === 'string' && value.trim()) {
      if (value !== currentJson) {
        setPatterns(newPatterns);
      }
    } else if (!value) {
      if (patterns.length > 0) {
        setPatterns([]);
      }
    }
    setJsonError('');
  }, [value]);

  useEffect(() => {
    if (editMode !== 'manual') {
      if (typeof value === 'string') setManualText(value);
      else setManualText('');
    }
  }, [value, editMode]);

  const patternsToJson = (pats) => {
    const cleaned = pats
      .filter((p) => p.match || p.replace_message)
      .map(({ match, match_type, replace_message }) => {
        const item = { match, match_type: match_type || 'contains' };
        if (replace_message) item.replace_message = replace_message;
        return item;
      });
    if (cleaned.length === 0) return '';
    return JSON.stringify({ patterns: cleaned }, null, 2);
  };

  const emitChange = useCallback(
    (jsonString) => {
      if (formApi && field) {
        formApi.setValue(field, jsonString);
      }
      onChange?.(jsonString);
    },
    [onChange, formApi, field],
  );

  const handleVisualChange = useCallback(
    (newPatterns) => {
      setPatterns(newPatterns);
      const jsonString = patternsToJson(newPatterns);
      setJsonError('');
      emitChange(jsonString);
    },
    [emitChange],
  );

  const handleManualChange = useCallback(
    (newValue) => {
      setManualText(newValue);
      if (newValue && newValue.trim()) {
        try {
          const parsed = JSON.parse(newValue);
          if (parsed && Array.isArray(parsed.patterns)) {
            setPatterns(
              parsed.patterns.map((p) => ({
                id: generateUniqueId(),
                match: p.match || '',
                match_type: p.match_type || 'contains',
                replace_message: p.replace_message || '',
              })),
            );
          }
          setJsonError('');
          emitChange(newValue);
        } catch (error) {
          setJsonError(error.message);
        }
      } else {
        setPatterns([]);
        setJsonError('');
        emitChange('');
      }
    },
    [emitChange],
  );

  const toggleEditMode = useCallback(() => {
    if (editMode === 'visual') {
      const jsonString = patternsToJson(patterns);
      setManualText(jsonString);
      setEditMode('manual');
    } else {
      try {
        if (manualText && manualText.trim()) {
          const parsed = JSON.parse(manualText);
          if (parsed && Array.isArray(parsed.patterns)) {
            setPatterns(
              parsed.patterns.map((p) => ({
                id: generateUniqueId(),
                match: p.match || '',
                match_type: p.match_type || 'contains',
                replace_message: p.replace_message || '',
              })),
            );
          }
        }
        setJsonError('');
        setEditMode('visual');
      } catch (error) {
        setJsonError(error.message);
      }
    }
  }, [editMode, manualText, patterns]);

  const addPattern = useCallback(() => {
    const newPatterns = [
      ...patterns,
      {
        id: generateUniqueId(),
        match: '',
        match_type: 'contains',
        replace_message: '',
      },
    ];
    handleVisualChange(newPatterns);
  }, [patterns, handleVisualChange]);

  const removePattern = useCallback(
    (id) => {
      const newPatterns = patterns.filter((p) => p.id !== id);
      handleVisualChange(newPatterns);
    },
    [patterns, handleVisualChange],
  );

  const updatePattern = useCallback(
    (id, fieldName, newValue) => {
      const newPatterns = patterns.map((p) =>
        p.id === id ? { ...p, [fieldName]: newValue } : p,
      );
      handleVisualChange(newPatterns);
    },
    [patterns, handleVisualChange],
  );

  const fillTemplate = useCallback(() => {
    const templateString = JSON.stringify(TEMPLATE, null, 2);
    if (formApi && field) {
      formApi.setValue(field, templateString);
    }
    setManualText(templateString);
    setPatterns(
      TEMPLATE.patterns.map((p) => ({
        id: generateUniqueId(),
        match: p.match,
        match_type: p.match_type,
        replace_message: p.replace_message,
      })),
    );
    onChange?.(templateString);
    setJsonError('');
  }, [onChange, formApi, field]);

  const renderVisualEditor = () => {
    return (
      <div className='space-y-1'>
        {patterns.length === 0 && (
          <div className='text-center py-6 px-4'>
            <Text type='tertiary' className='text-gray-500 text-sm'>
              {t('暂无规则，点击下方按钮添加')}
            </Text>
          </div>
        )}

        {patterns.map((pattern) => (
          <Row key={pattern.id} gutter={8} align='middle'>
            <Col span={8}>
              <Input
                placeholder={t('匹配内容')}
                value={pattern.match}
                onChange={(val) => updatePattern(pattern.id, 'match', val)}
              />
            </Col>
            <Col span={5}>
              <Select
                value={pattern.match_type}
                onChange={(val) =>
                  updatePattern(pattern.id, 'match_type', val)
                }
                style={{ width: '100%' }}
              >
                {MATCH_TYPE_OPTIONS.map((opt) => (
                  <Select.Option key={opt.value} value={opt.value}>
                    {t(opt.labelKey)}
                  </Select.Option>
                ))}
              </Select>
            </Col>
            <Col span={9}>
              <Input
                placeholder={t('替换为')}
                value={pattern.replace_message}
                onChange={(val) =>
                  updatePattern(pattern.id, 'replace_message', val)
                }
              />
            </Col>
            <Col span={2}>
              <Button
                icon={<IconDelete />}
                type='danger'
                theme='borderless'
                onClick={() => removePattern(pattern.id)}
                style={{ width: '100%' }}
              />
            </Col>
          </Row>
        ))}

        <div className='mt-2 flex justify-center'>
          <Button
            icon={<IconPlus />}
            type='primary'
            theme='outline'
            onClick={addPattern}
          >
            {t('添加规则')}
          </Button>
        </div>
      </div>
    );
  };

  const hasJsonError = jsonError && jsonError.trim() !== '';

  return (
    <Form.Slot label={label}>
      <Card
        header={
          <div className='flex justify-between items-center'>
            <Tabs
              type='slash'
              activeKey={editMode}
              onChange={(key) => {
                if (key === 'manual' && editMode === 'visual') {
                  setEditMode('manual');
                  const jsonString = patternsToJson(patterns);
                  setManualText(jsonString);
                } else if (key === 'visual' && editMode === 'manual') {
                  toggleEditMode();
                }
              }}
            >
              <TabPane tab={t('可视化')} itemKey='visual' />
              <TabPane tab={t('手动编辑')} itemKey='manual' />
            </Tabs>

            <Button type='tertiary' onClick={fillTemplate} size='small'>
              {t('填入模板')}
            </Button>
          </div>
        }
        headerStyle={{ padding: '12px 16px' }}
        bodyStyle={{ padding: '16px' }}
        className='!rounded-2xl'
      >
        {hasJsonError && (
          <Banner
            type='danger'
            description={`JSON ${t('格式错误')}: ${jsonError}`}
            className='mb-3'
          />
        )}

        {editMode === 'visual' ? (
          <div>
            {renderVisualEditor()}
            <Form.Input
              field={field}
              value={value}
              style={{ display: 'none' }}
              noLabel={true}
            />
          </div>
        ) : (
          <div>
            <TextArea
              placeholder={JSON.stringify(TEMPLATE, null, 2)}
              value={manualText}
              onChange={handleManualChange}
              showClear
              rows={Math.max(8, manualText ? manualText.split('\n').length : 8)}
            />
            <Form.Input
              field={field}
              value={value}
              style={{ display: 'none' }}
              noLabel={true}
            />
          </div>
        )}

        <Divider margin='12px' align='center'>
          <Text type='tertiary' size='small'>
            {t('第一个匹配的规则生效，支持重写 message、type、code 字段')}
          </Text>
        </Divider>
      </Card>
    </Form.Slot>
  );
};

export default ErrorMappingEditor;
