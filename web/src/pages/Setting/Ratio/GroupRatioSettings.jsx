/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useEffect, useState, useRef, useMemo, useCallback } from 'react';
import {
  Table,
  Button,
  Input,
  InputNumber,
  Modal,
  Form,
  Switch,
  Checkbox,
  Tag,
  RadioGroup,
  Radio,
  Space,
  Popconfirm,
  Typography,
  Select,
  Spin,
  Col,
  Row,
} from '@douyinfe/semi-ui';
import {
  IconDelete,
  IconPlus,
  IconEdit,
  IconArrowUp,
  IconArrowDown,
  IconSearch,
} from '@douyinfe/semi-icons';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
  verifyJSON,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

const { Text } = Typography;

// Wrapper component to handle IME (Chinese/Japanese/Korean input) correctly.
// Prevents parent re-render from interrupting IME composition.
function IMEInput({ value: externalValue, onChange: onExternalChange, ...props }) {
  const [value, setValue] = useState(externalValue ?? '');
  const composingRef = useRef(false);

  useEffect(() => {
    if (!composingRef.current) {
      setValue(externalValue ?? '');
    }
  }, [externalValue]);

  return (
    <Input
      value={value}
      onChange={(val) => {
        setValue(val);
        if (!composingRef.current) {
          onExternalChange(val);
        }
      }}
      onCompositionStart={() => {
        composingRef.current = true;
      }}
      onCompositionEnd={(e) => {
        composingRef.current = false;
        const finalValue = e.target.value;
        setValue(finalValue);
        onExternalChange(finalValue);
      }}
      {...props}
    />
  );
}

// Parse the 5 option strings into a unified groups array
function parseOptionsToGroups(options) {
  const groupRatio = JSON.parse(options.GroupRatio || '{}');
  const userUsableGroups = JSON.parse(options.UserUsableGroups || '{}');
  const groupGroupRatio = JSON.parse(options.GroupGroupRatio || '{}');
  const specialUsableGroup = JSON.parse(
    options['group_ratio_setting.group_special_usable_group'] || '{}'
  );
  const autoGroups = JSON.parse(options.AutoGroups || '[]');

  return Object.entries(groupRatio).map(([name, ratio]) => ({
    name,
    ratio: typeof ratio === 'number' ? ratio : parseFloat(ratio) || 0,
    description: userUsableGroups[name] || '',
    userSelectable: name in userUsableGroups,
    autoGroupOrder: autoGroups.indexOf(name),
    specialRatios: groupGroupRatio[name] || {},
    specialUsableGroups: specialUsableGroup[name] || {},
  }));
}

// Serialize groups array back into the 5 option strings
function groupsToOptions(groups) {
  const result = {
    GroupRatio: {},
    UserUsableGroups: {},
    GroupGroupRatio: {},
    'group_ratio_setting.group_special_usable_group': {},
    AutoGroups: [],
  };

  // AutoGroups: sorted by order
  groups
    .filter((g) => g.autoGroupOrder >= 0)
    .sort((a, b) => a.autoGroupOrder - b.autoGroupOrder)
    .forEach((g) => result.AutoGroups.push(g.name));

  groups.forEach((g) => {
    result.GroupRatio[g.name] = g.ratio;
    if (g.userSelectable) {
      result.UserUsableGroups[g.name] = g.description || g.name;
    }
    if (Object.keys(g.specialRatios).length > 0) {
      result.GroupGroupRatio[g.name] = g.specialRatios;
    }
    if (Object.keys(g.specialUsableGroups).length > 0) {
      result['group_ratio_setting.group_special_usable_group'][g.name] =
        g.specialUsableGroups;
    }
  });

  return Object.fromEntries(
    Object.entries(result).map(([k, v]) => [k, JSON.stringify(v, null, 2)])
  );
}

export default function GroupRatioSettings(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [editMode, setEditMode] = useState('visual'); // 'visual' | 'json'
  const [groups, setGroups] = useState([]);
  const [defaultUseAutoGroup, setDefaultUseAutoGroup] = useState(false);
  const [searchText, setSearchText] = useState('');
  const [currentPage, setCurrentPage] = useState(1);
  const pageSize = 10;

  // JSON mode state
  const [jsonInputs, setJsonInputs] = useState({
    GroupRatio: '',
    UserUsableGroups: '',
    GroupGroupRatio: '',
    'group_ratio_setting.group_special_usable_group': '',
    AutoGroups: '',
  });
  const jsonFormRef = useRef();

  // Initial state for dirty checking
  const [initialOptionStrings, setInitialOptionStrings] = useState({});

  // Add group modal
  const [addModalVisible, setAddModalVisible] = useState(false);
  const [newGroupName, setNewGroupName] = useState('');
  const [newGroupRatio, setNewGroupRatio] = useState(1);

  // Advanced settings modal
  const [advancedModalVisible, setAdvancedModalVisible] = useState(false);
  const [advancedGroup, setAdvancedGroup] = useState(null);
  const [editSpecialRatios, setEditSpecialRatios] = useState([]);
  const [editSpecialUsableGroups, setEditSpecialUsableGroups] = useState([]);

  // Parse options on mount/update
  useEffect(() => {
    if (!props.options) return;
    try {
      const parsed = parseOptionsToGroups(props.options);
      setGroups(parsed);

      const defaultAutoGroup = props.options.DefaultUseAutoGroup;
      setDefaultUseAutoGroup(
        defaultAutoGroup === true ||
          defaultAutoGroup === 'true' ||
          defaultAutoGroup === '1'
      );

      // Set initial JSON inputs
      const optionStrings = {
        GroupRatio: props.options.GroupRatio || '{}',
        UserUsableGroups: props.options.UserUsableGroups || '{}',
        GroupGroupRatio: props.options.GroupGroupRatio || '{}',
        'group_ratio_setting.group_special_usable_group':
          props.options['group_ratio_setting.group_special_usable_group'] || '{}',
        AutoGroups: props.options.AutoGroups || '[]',
      };
      setJsonInputs(optionStrings);
      // If currently in JSON mode, sync form values
      if (jsonFormRef.current) {
        jsonFormRef.current.setValues(optionStrings);
      }
      setInitialOptionStrings({
        ...optionStrings,
        DefaultUseAutoGroup: String(
          defaultAutoGroup === true ||
            defaultAutoGroup === 'true' ||
            defaultAutoGroup === '1'
        ),
      });
    } catch (error) {
      console.error('Failed to parse group options:', error);
    }
  }, [props.options]);

  // Sync: when switching from visual → json, serialize groups to JSON
  // When switching from json → visual, parse JSON to groups
  const handleEditModeChange = useCallback(
    (e) => {
      const newMode = e.target.value;
      if (newMode === editMode) return;

      if (newMode === 'json') {
        // visual → json: serialize current groups
        const optionStrings = groupsToOptions(groups);
        setJsonInputs(optionStrings);
        // Form hasn't mounted yet at this point, use setTimeout to set values
        // after React renders the JSON form (same pattern as ModelSettingsVisualEditor)
        setEditMode(newMode);
        setTimeout(() => {
          if (jsonFormRef.current) {
            jsonFormRef.current.setValues(optionStrings);
          }
        }, 0);
        return;
      } else {
        // json → visual: parse JSON texts
        try {
          for (const [key, val] of Object.entries(jsonInputs)) {
            if (val && val.trim() !== '' && !verifyJSON(val)) {
              showError(t('JSON 格式错误，请检查后再切换'));
              return;
            }
          }
          const mockOptions = { ...jsonInputs };
          const parsed = parseOptionsToGroups(mockOptions);
          setGroups(parsed);
        } catch (error) {
          showError(t('JSON 解析失败') + ': ' + error.message);
          return;
        }
      }
      setEditMode(newMode);
    },
    [editMode, groups, jsonInputs, t]
  );

  // Group manipulation helpers
  const updateGroup = useCallback((name, field, value) => {
    setGroups((prev) =>
      prev.map((g) => (g.name === name ? { ...g, [field]: value } : g))
    );
  }, []);

  const handleUserSelectableChange = useCallback((name, checked) => {
    setGroups((prev) =>
      prev.map((g) => {
        if (g.name !== name) return g;
        return { ...g, userSelectable: checked };
      })
    );
  }, []);

  const handleAutoGroupChange = useCallback((name, checked) => {
    setGroups((prev) => {
      const updated = prev.map((g) => {
        if (g.name !== name) return g;
        if (checked) {
          // Add to auto groups at the end
          const maxOrder = Math.max(-1, ...prev.filter((x) => x.autoGroupOrder >= 0).map((x) => x.autoGroupOrder));
          return { ...g, autoGroupOrder: maxOrder + 1 };
        } else {
          return { ...g, autoGroupOrder: -1 };
        }
      });
      // Re-index auto group orders
      return reindexAutoGroups(updated);
    });
  }, []);

  const reindexAutoGroups = (groupList) => {
    const autoItems = groupList
      .filter((g) => g.autoGroupOrder >= 0)
      .sort((a, b) => a.autoGroupOrder - b.autoGroupOrder);
    const orderMap = {};
    autoItems.forEach((g, idx) => {
      orderMap[g.name] = idx;
    });
    return groupList.map((g) =>
      g.autoGroupOrder >= 0 ? { ...g, autoGroupOrder: orderMap[g.name] } : g
    );
  };

  const moveAutoGroup = useCallback((name, direction) => {
    setGroups((prev) => {
      const autoItems = prev
        .filter((g) => g.autoGroupOrder >= 0)
        .sort((a, b) => a.autoGroupOrder - b.autoGroupOrder);
      const idx = autoItems.findIndex((g) => g.name === name);
      if (idx < 0) return prev;
      const targetIdx = idx + direction;
      if (targetIdx < 0 || targetIdx >= autoItems.length) return prev;

      // Swap orders
      const temp = autoItems[idx].autoGroupOrder;
      const orderMap = {};
      autoItems.forEach((g, i) => {
        if (i === idx) orderMap[g.name] = autoItems[targetIdx].autoGroupOrder;
        else if (i === targetIdx) orderMap[g.name] = temp;
        else orderMap[g.name] = g.autoGroupOrder;
      });
      return prev.map((g) =>
        orderMap[g.name] !== undefined
          ? { ...g, autoGroupOrder: orderMap[g.name] }
          : g
      );
    });
  }, []);

  const deleteGroup = useCallback((name) => {
    setGroups((prev) => reindexAutoGroups(prev.filter((g) => g.name !== name)));
  }, []);

  const addGroup = useCallback(() => {
    const name = newGroupName.trim();
    if (!name) {
      showError(t('分组名称不能为空'));
      return;
    }
    if (groups.some((g) => g.name === name)) {
      showError(t('分组名称已存在'));
      return;
    }
    setGroups((prev) => [
      ...prev,
      {
        name,
        ratio: newGroupRatio,
        description: '',
        userSelectable: false,
        autoGroupOrder: -1,
        specialRatios: {},
        specialUsableGroups: {},
      },
    ]);
    setNewGroupName('');
    setNewGroupRatio(1);
    setAddModalVisible(false);
    showSuccess(t('添加成功'));
  }, [newGroupName, newGroupRatio, groups, t]);

  // Advanced settings modal logic
  const openAdvancedModal = useCallback(
    (group) => {
      setAdvancedGroup(group);
      // Convert specialRatios object to editable array
      setEditSpecialRatios(
        Object.entries(group.specialRatios).map(([targetGroup, ratio]) => ({
          targetGroup,
          ratio: typeof ratio === 'number' ? ratio : parseFloat(ratio) || 0,
        }))
      );
      // Convert specialUsableGroups object to editable array
      setEditSpecialUsableGroups(
        Object.entries(group.specialUsableGroups).map(([key, desc]) => {
          let actionType = 'add'; // default
          let groupName = key;
          if (key.startsWith('+:')) {
            actionType = 'add';
            groupName = key.substring(2);
          } else if (key.startsWith('-:')) {
            actionType = 'remove';
            groupName = key.substring(2);
          }
          return { actionType, groupName, description: desc };
        })
      );
      setAdvancedModalVisible(true);
    },
    []
  );

  const saveAdvancedSettings = useCallback(() => {
    if (!advancedGroup) return;

    // Validate: no duplicate target groups in special ratios
    const ratioTargets = editSpecialRatios.map((r) => r.targetGroup);
    if (new Set(ratioTargets).size !== ratioTargets.length) {
      showError(t('特殊倍率中存在重复的分组'));
      return;
    }

    // Validate: no empty group names
    if (editSpecialRatios.some((r) => !r.targetGroup.trim())) {
      showError(t('分组名称不能为空'));
      return;
    }
    if (editSpecialUsableGroups.some((r) => !r.groupName.trim())) {
      showError(t('分组名称不能为空'));
      return;
    }

    // Build specialRatios object
    const newSpecialRatios = {};
    editSpecialRatios.forEach((r) => {
      newSpecialRatios[r.targetGroup.trim()] = r.ratio;
    });

    // Build specialUsableGroups object
    const newSpecialUsableGroups = {};
    editSpecialUsableGroups.forEach((r) => {
      const name = r.groupName.trim();
      const key = r.actionType === 'remove' ? `-:${name}` : `+:${name}`;
      newSpecialUsableGroups[key] = r.description || '';
    });

    setGroups((prev) =>
      prev.map((g) =>
        g.name === advancedGroup.name
          ? {
              ...g,
              specialRatios: newSpecialRatios,
              specialUsableGroups: newSpecialUsableGroups,
            }
          : g
      )
    );
    setAdvancedModalVisible(false);
    setAdvancedGroup(null);
  }, [advancedGroup, editSpecialRatios, editSpecialUsableGroups, t]);

  // Save handler
  const onSubmit = useCallback(async () => {
    let currentOptionStrings;
    if (editMode === 'visual') {
      currentOptionStrings = groupsToOptions(groups);
    } else {
      // Validate JSON inputs
      for (const [key, val] of Object.entries(jsonInputs)) {
        if (val && val.trim() !== '' && !verifyJSON(val)) {
          showError(t('不是合法的 JSON 字符串') + `: ${key}`);
          return;
        }
      }
      currentOptionStrings = { ...jsonInputs };
    }

    // Add DefaultUseAutoGroup
    const allCurrent = {
      ...currentOptionStrings,
      DefaultUseAutoGroup: String(defaultUseAutoGroup),
    };

    const updateArray = compareObjects(initialOptionStrings, allCurrent);
    if (!updateArray.length) {
      showWarning(t('你似乎并没有修改什么'));
      return;
    }

    const requestQueue = updateArray.map((item) =>
      API.put('/api/option/', { key: item.key, value: allCurrent[item.key] })
    );

    setLoading(true);
    try {
      const res = await Promise.all(requestQueue);
      if (res.includes(undefined)) {
        showError(
          requestQueue.length > 1
            ? t('部分保存失败，请重试')
            : t('保存失败')
        );
        return;
      }
      for (let i = 0; i < res.length; i++) {
        if (!res[i].data.success) {
          showError(res[i].data.message);
          return;
        }
      }
      showSuccess(t('保存成功'));
      props.refresh();
    } catch (error) {
      console.error('Save failed:', error);
      showError(t('保存失败，请重试'));
    } finally {
      setLoading(false);
    }
  }, [editMode, groups, jsonInputs, defaultUseAutoGroup, initialOptionStrings, t, props]);

  // Filtered and paged data for visual mode
  const filteredGroups = useMemo(() => {
    if (!searchText) return groups;
    const lower = searchText.toLowerCase();
    return groups.filter(
      (g) =>
        g.name.toLowerCase().includes(lower) ||
        g.description.toLowerCase().includes(lower)
    );
  }, [groups, searchText]);

  const pagedGroups = useMemo(() => {
    const start = (currentPage - 1) * pageSize;
    return filteredGroups.slice(start, start + pageSize);
  }, [filteredGroups, currentPage, pageSize]);

  // All group names for select dropdowns in advanced modal
  const allGroupNames = useMemo(
    () => groups.map((g) => g.name),
    [groups]
  );

  // Table columns
  const columns = useMemo(() => [
    {
      title: t('分组名称'),
      dataIndex: 'name',
      key: 'name',
      width: 140,
      render: (text, record) => (
        <Text strong copyable>
          {text}
        </Text>
      ),
    },
    {
      title: t('倍率'),
      dataIndex: 'ratio',
      key: 'ratio',
      width: 120,
      render: (_, record) => (
        <InputNumber
          value={record.ratio}
          min={0}
          hideButtons
          style={{ width: '100%' }}
          onChange={(value) => updateGroup(record.name, 'ratio', value ?? 0)}
        />
      ),
    },
    {
      title: t('描述'),
      dataIndex: 'description',
      key: 'description',
      width: 180,
      render: (_, record) => (
        <IMEInput
          value={record.description}
          placeholder={record.userSelectable ? t('输入分组描述') : '-'}
          disabled={!record.userSelectable}
          onChange={(value) => updateGroup(record.name, 'description', value)}
        />
      ),
    },
    {
      title: t('用户可选'),
      dataIndex: 'userSelectable',
      key: 'userSelectable',
      width: 90,
      align: 'center',
      render: (_, record) => (
        <Checkbox
          checked={record.userSelectable}
          onChange={(e) =>
            handleUserSelectableChange(record.name, e.target.checked)
          }
        />
      ),
    },
    {
      title: t('自动分组'),
      dataIndex: 'autoGroupOrder',
      key: 'autoGroupOrder',
      width: 140,
      align: 'center',
      render: (_, record) => (
        <Space>
          <Checkbox
            checked={record.autoGroupOrder >= 0}
            onChange={(e) =>
              handleAutoGroupChange(record.name, e.target.checked)
            }
          />
          {record.autoGroupOrder >= 0 && (
            <>
              <Tag color='blue' size='small'>
                {record.autoGroupOrder + 1}
              </Tag>
              <Button
                icon={<IconArrowUp />}
                size='small'
                theme='borderless'
                onClick={() => moveAutoGroup(record.name, -1)}
                disabled={record.autoGroupOrder === 0}
              />
              <Button
                icon={<IconArrowDown />}
                size='small'
                theme='borderless'
                onClick={() => moveAutoGroup(record.name, 1)}
                disabled={
                  record.autoGroupOrder >=
                  groups.filter((g) => g.autoGroupOrder >= 0).length - 1
                }
              />
            </>
          )}
        </Space>
      ),
    },
    {
      title: t('操作'),
      key: 'action',
      width: 120,
      render: (_, record) => {
        const hasAdvanced =
          Object.keys(record.specialRatios).length > 0 ||
          Object.keys(record.specialUsableGroups).length > 0;
        return (
          <Space>
            <Button
              icon={<IconEdit />}
              size='small'
              type={hasAdvanced ? 'primary' : 'tertiary'}
              onClick={() => openAdvancedModal(record)}
            />
            <Popconfirm
              title={t('确认删除') + ` "${record.name}"?`}
              onConfirm={() => deleteGroup(record.name)}
              okText={t('确认')}
              cancelText={t('取消')}
            >
              <Button icon={<IconDelete />} size='small' type='danger' />
            </Popconfirm>
          </Space>
        );
      },
    },
  ], [t, groups, updateGroup, handleUserSelectableChange, handleAutoGroupChange, moveAutoGroup, openAdvancedModal, deleteGroup]);

  return (
    <Spin spinning={loading}>
      <Space
        vertical
        align='start'
        style={{ width: '100%', marginBottom: 15 }}
      >
        {/* Mode toggle */}
        <RadioGroup
          type='button'
          value={editMode}
          onChange={handleEditModeChange}
          style={{ marginBottom: 12 }}
        >
          <Radio value='visual'>{t('可视化编辑')}</Radio>
          <Radio value='json'>{t('JSON 编辑')}</Radio>
        </RadioGroup>

        {editMode === 'visual' ? (
          <>
            {/* Toolbar */}
            <Space style={{ marginBottom: 8 }}>
              <Button
                icon={<IconPlus />}
                onClick={() => setAddModalVisible(true)}
              >
                {t('添加分组')}
              </Button>
              <IMEInput
                prefix={<IconSearch />}
                placeholder={t('搜索分组')}
                value={searchText}
                onChange={(value) => {
                  setSearchText(value);
                  setCurrentPage(1);
                }}
                style={{ width: 200 }}
                showClear
              />
            </Space>

            {/* Main table */}
            <Table
              columns={columns}
              dataSource={pagedGroups}
              rowKey='name'
              pagination={{
                currentPage,
                pageSize,
                total: filteredGroups.length,
                onPageChange: (page) => setCurrentPage(page),
                showTotal: true,
                showSizeChanger: false,
              }}
              size='small'
              style={{ width: '100%' }}
            />

            {/* DefaultUseAutoGroup switch */}
            <div style={{ marginTop: 8 }}>
              <Switch
                checked={defaultUseAutoGroup}
                onChange={(val) => setDefaultUseAutoGroup(val)}
              />
              <Text style={{ marginLeft: 8 }}>
                {t(
                  '创建令牌默认选择auto分组，初始令牌也将设为auto（否则留空，为用户默认分组）'
                )}
              </Text>
            </div>
          </>
        ) : (
          /* JSON editing mode - preserves original 5 text areas */
          <>
          <Form
            values={jsonInputs}
            getFormApi={(formAPI) => (jsonFormRef.current = formAPI)}
          >
            <Row gutter={16}>
              <Col xs={24} sm={16}>
                <Form.TextArea
                  label={t('分组倍率')}
                  placeholder={t('为一个 JSON 文本，键为分组名称，值为倍率')}
                  extraText={t(
                    '分组倍率设置，可以在此处新增分组或修改现有分组的倍率，格式为 JSON 字符串，例如：{"vip": 0.5, "test": 1}，表示 vip 分组的倍率为 0.5，test 分组的倍率为 1'
                  )}
                  field={'GroupRatio'}
                  autosize={{ minRows: 6, maxRows: 12 }}
                  trigger='blur'
                  stopValidateWithError
                  rules={[
                    {
                      validator: (rule, value) => verifyJSON(value),
                      message: t('不是合法的 JSON 字符串'),
                    },
                  ]}
                  onChange={(value) =>
                    setJsonInputs((prev) => ({ ...prev, GroupRatio: value }))
                  }
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={16}>
                <Form.TextArea
                  label={t('用户可选分组')}
                  placeholder={t(
                    '为一个 JSON 文本，键为分组名称，值为分组描述'
                  )}
                  extraText={t(
                    '用户新建令牌时可选的分组，格式为 JSON 字符串，例如：{"vip": "VIP 用户", "test": "测试"}，表示用户可以选择 vip 分组和 test 分组'
                  )}
                  field={'UserUsableGroups'}
                  autosize={{ minRows: 6, maxRows: 12 }}
                  trigger='blur'
                  stopValidateWithError
                  rules={[
                    {
                      validator: (rule, value) => verifyJSON(value),
                      message: t('不是合法的 JSON 字符串'),
                    },
                  ]}
                  onChange={(value) =>
                    setJsonInputs((prev) => ({
                      ...prev,
                      UserUsableGroups: value,
                    }))
                  }
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={16}>
                <Form.TextArea
                  label={t('分组特殊倍率')}
                  placeholder={t('为一个 JSON 文本')}
                  extraText={t(
                    '键为分组名称，值为另一个 JSON 对象，键为分组名称，值为该分组的用户的特殊分组倍率，例如：{"vip": {"default": 0.5, "test": 1}}，表示 vip 分组的用户在使用default分组的令牌时倍率为0.5，使用test分组时倍率为1'
                  )}
                  field={'GroupGroupRatio'}
                  autosize={{ minRows: 6, maxRows: 12 }}
                  trigger='blur'
                  stopValidateWithError
                  rules={[
                    {
                      validator: (rule, value) => verifyJSON(value),
                      message: t('不是合法的 JSON 字符串'),
                    },
                  ]}
                  onChange={(value) =>
                    setJsonInputs((prev) => ({
                      ...prev,
                      GroupGroupRatio: value,
                    }))
                  }
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={16}>
                <Form.TextArea
                  label={t('分组特殊可用分组')}
                  placeholder={t('为一个 JSON 文本')}
                  extraText={t(
                    '键为用户分组名称，值为操作映射对象。内层键以"+:"开头表示添加指定分组（键值为分组名称，值为描述），以"-:"开头表示移除指定分组（键值为分组名称），不带前缀的键直接添加该分组。例如：{"vip": {"+:premium": "高级分组", "special": "特殊分组", "-:default": "默认分组"}}，表示 vip 分组的用户可以使用 premium 和 special 分组，同时移除 default 分组的访问权限'
                  )}
                  field={'group_ratio_setting.group_special_usable_group'}
                  autosize={{ minRows: 6, maxRows: 12 }}
                  trigger='blur'
                  stopValidateWithError
                  rules={[
                    {
                      validator: (rule, value) => verifyJSON(value),
                      message: t('不是合法的 JSON 字符串'),
                    },
                  ]}
                  onChange={(value) =>
                    setJsonInputs((prev) => ({
                      ...prev,
                      'group_ratio_setting.group_special_usable_group': value,
                    }))
                  }
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={16}>
                <Form.TextArea
                  label={t('自动分组auto，从第一个开始选择')}
                  placeholder={t('为一个 JSON 文本')}
                  field={'AutoGroups'}
                  autosize={{ minRows: 6, maxRows: 12 }}
                  trigger='blur'
                  stopValidateWithError
                  rules={[
                    {
                      validator: (rule, value) => {
                        if (!value || value.trim() === '') return true;
                        try {
                          const parsed = JSON.parse(value);
                          if (!Array.isArray(parsed)) return false;
                          return parsed.every(
                            (item) => typeof item === 'string'
                          );
                        } catch (error) {
                          return false;
                        }
                      },
                      message: t(
                        '必须是有效的 JSON 字符串数组，例如：["g1","g2"]'
                      ),
                    },
                  ]}
                  onChange={(value) =>
                    setJsonInputs((prev) => ({ ...prev, AutoGroups: value }))
                  }
                />
              </Col>
            </Row>
          </Form>
          <div style={{ marginTop: 8 }}>
            <Switch
              checked={defaultUseAutoGroup}
              onChange={(val) => setDefaultUseAutoGroup(val)}
            />
            <Text style={{ marginLeft: 8 }}>
              {t(
                '创建令牌默认选择auto分组，初始令牌也将设为auto（否则留空，为用户默认分组）'
              )}
            </Text>
          </div>
          </>
        )}

        <Button onClick={onSubmit} type='primary' style={{ marginTop: 8 }}>
          {t('保存分组倍率设置')}
        </Button>
      </Space>

      {/* Add Group Modal */}
      <Modal
        title={t('添加分组')}
        visible={addModalVisible}
        onCancel={() => {
          setAddModalVisible(false);
          setNewGroupName('');
          setNewGroupRatio(1);
        }}
        onOk={addGroup}
        okText={t('确认')}
        cancelText={t('取消')}
      >
        <Form layout='vertical'>
          <Form.Slot label={t('分组名称')}>
            <IMEInput
              value={newGroupName}
              placeholder={t('输入分组名称')}
              onChange={(value) => setNewGroupName(value)}
            />
          </Form.Slot>
          <Form.Slot label={t('倍率')}>
            <InputNumber
              value={newGroupRatio}
              min={0}
              hideButtons
              onChange={(value) => setNewGroupRatio(value ?? 1)}
              style={{ width: '100%' }}
            />
          </Form.Slot>
        </Form>
      </Modal>

      {/* Advanced Settings Modal */}
      <Modal
        title={
          advancedGroup
            ? t('编辑分组高级设置') + ': ' + advancedGroup.name
            : ''
        }
        visible={advancedModalVisible}
        onCancel={() => {
          setAdvancedModalVisible(false);
          setAdvancedGroup(null);
        }}
        onOk={saveAdvancedSettings}
        okText={t('确认')}
        cancelText={t('取消')}
        width={640}
      >
        {advancedGroup && (
          <Space vertical align='start' style={{ width: '100%' }}>
            {/* Special Ratios Section */}
            <div style={{ width: '100%' }}>
              <Text strong style={{ display: 'block', marginBottom: 4 }}>
                {t('特殊倍率规则')}
              </Text>
              <Text
                type='tertiary'
                size='small'
                style={{ display: 'block', marginBottom: 8 }}
              >
                {t('该分组用户使用其他分组令牌时的特殊倍率')}
              </Text>

              {editSpecialRatios.map((item, index) => (
                <div
                  key={index}
                  style={{ display: 'flex', gap: 8, marginBottom: 8, alignItems: 'center' }}
                >
                  <Select
                    value={item.targetGroup}
                    placeholder={t('使用分组')}
                    style={{ flex: 1, minWidth: 0 }}
                    filter
                    onChange={(value) => {
                      const updated = [...editSpecialRatios];
                      updated[index] = { ...updated[index], targetGroup: value };
                      setEditSpecialRatios(updated);
                    }}
                  >
                    {allGroupNames
                      .filter((name) => name !== advancedGroup?.name)
                      .map((name) => (
                        <Select.Option key={name} value={name}>
                          {name}
                        </Select.Option>
                      ))}
                  </Select>
                  <InputNumber
                    value={item.ratio}
                    min={0}
                    hideButtons
                    style={{ width: 100, flexShrink: 0 }}
                    onChange={(value) => {
                      const updated = [...editSpecialRatios];
                      updated[index] = {
                        ...updated[index],
                        ratio: value ?? 0,
                      };
                      setEditSpecialRatios(updated);
                    }}
                  />
                  <Button
                    icon={<IconDelete />}
                    type='danger'
                    size='small'
                    style={{ flexShrink: 0 }}
                    onClick={() => {
                      setEditSpecialRatios((prev) =>
                        prev.filter((_, i) => i !== index)
                      );
                    }}
                  />
                </div>
              ))}
              <Button
                icon={<IconPlus />}
                size='small'
                onClick={() =>
                  setEditSpecialRatios((prev) => [
                    ...prev,
                    { targetGroup: '', ratio: 1 },
                  ])
                }
              >
                {t('添加规则')}
              </Button>
            </div>

            {/* Special Usable Groups Section */}
            <div style={{ width: '100%', marginTop: 16 }}>
              <Text strong style={{ display: 'block', marginBottom: 4 }}>
                {t('特殊可用分组')}
              </Text>
              <Text
                type='tertiary'
                size='small'
                style={{ display: 'block', marginBottom: 8 }}
              >
                {t('该分组用户额外可用或不可用的分组')}
              </Text>

              {editSpecialUsableGroups.map((item, index) => (
                <div
                  key={index}
                  style={{ display: 'flex', gap: 8, marginBottom: 8, alignItems: 'center' }}
                >
                  <Select
                    value={item.actionType}
                    style={{ width: 100, flexShrink: 0 }}
                    onChange={(value) => {
                      const updated = [...editSpecialUsableGroups];
                      updated[index] = { ...updated[index], actionType: value };
                      setEditSpecialUsableGroups(updated);
                    }}
                  >
                    <Select.Option value='add'>
                      {t('添加')} (+)
                    </Select.Option>
                    <Select.Option value='remove'>
                      {t('移除')} (-)
                    </Select.Option>
                  </Select>
                  <Select
                    value={item.groupName}
                    placeholder={t('分组名称')}
                    style={{ flex: 1, minWidth: 0 }}
                    filter
                    onChange={(value) => {
                      const updated = [...editSpecialUsableGroups];
                      updated[index] = { ...updated[index], groupName: value };
                      setEditSpecialUsableGroups(updated);
                    }}
                  >
                    {allGroupNames.map((name) => (
                      <Select.Option key={name} value={name}>
                        {name}
                      </Select.Option>
                    ))}
                  </Select>
                  <IMEInput
                    value={item.description}
                    placeholder={t('描述')}
                    style={{ flex: 1, minWidth: 0 }}
                    onChange={(value) => {
                      const updated = [...editSpecialUsableGroups];
                      updated[index] = {
                        ...updated[index],
                        description: value,
                      };
                      setEditSpecialUsableGroups(updated);
                    }}
                  />
                  <Button
                    icon={<IconDelete />}
                    type='danger'
                    size='small'
                    style={{ flexShrink: 0 }}
                    onClick={() => {
                      setEditSpecialUsableGroups((prev) =>
                        prev.filter((_, i) => i !== index)
                      );
                    }}
                  />
                </div>
              ))}
              <Button
                icon={<IconPlus />}
                size='small'
                onClick={() =>
                  setEditSpecialUsableGroups((prev) => [
                    ...prev,
                    { actionType: 'add', groupName: '', description: '' },
                  ])
                }
              >
                {t('添加规则')}
              </Button>
            </div>
          </Space>
        )}
      </Modal>
    </Spin>
  );
}
