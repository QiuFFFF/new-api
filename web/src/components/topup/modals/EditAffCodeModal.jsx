import React, { useState, useEffect } from 'react';
import { Modal, Typography, Input } from '@douyinfe/semi-ui';
import { Edit3 } from 'lucide-react';

const EditAffCodeModal = ({
  t,
  visible,
  onOk,
  onCancel,
  confirmLoading,
}) => {
  const [affCode, setAffCode] = useState('');
  const [error, setError] = useState('');

  useEffect(() => {
    if (visible) {
      setAffCode('');
      setError('');
    }
  }, [visible]);

  const handleChange = (value) => {
    const filtered = value.replace(/[^a-zA-Z0-9]/g, '').toUpperCase();
    setAffCode(filtered.slice(0, 4));
    setError('');
  };

  const handleOk = () => {
    if (affCode.length !== 4) {
      setError(t('邀请码必须为4位字母或数字'));
      return;
    }
    if (!/^[A-Za-z0-9]{4}$/.test(affCode)) {
      setError(t('邀请码只能包含字母和数字'));
      return;
    }
    onOk(affCode);
  };

  return (
    <Modal
      title={
        <div className='flex items-center'>
          <Edit3 className='mr-2' size={18} />
          {t('自定义邀请码')}
        </div>
      }
      visible={visible}
      onOk={handleOk}
      onCancel={onCancel}
      maskClosable={false}
      centered
      confirmLoading={confirmLoading}
      okText={t('确认修改')}
    >
      <div className='space-y-4'>
        <div>
          <Typography.Text strong className='block mb-2'>
            {t('新邀请码')}
          </Typography.Text>
          <Input
            value={affCode}
            onChange={handleChange}
            placeholder={t('请输入4位字母或数字')}
            maxLength={4}
            className='!rounded-lg'
            validateStatus={error ? 'error' : undefined}
          />
          {error && (
            <Typography.Text type='danger' className='text-xs mt-1 block'>
              {error}
            </Typography.Text>
          )}
        </div>
        <Typography.Text type='tertiary' className='text-xs'>
          {t('邀请码将自动转换为大写，修改后原邀请链接将失效')}
        </Typography.Text>
      </div>
    </Modal>
  );
};

export default EditAffCodeModal;
