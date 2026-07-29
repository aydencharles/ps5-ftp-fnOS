import type { FormRule } from 'tdesign-vue-next'

export const optionalPasswordRules: FormRule[] = [
  { max: 1024, message: '密码不能超过 1024 个字符', trigger: 'blur' },
]

export const remoteEntryNameRules: FormRule[] = [
  { required: true, whitespace: true, message: '请输入名称', trigger: 'blur' },
  { max: 255, message: '名称不能超过 255 个字符', trigger: 'blur' },
  {
    validator: (value) => typeof value === 'string' && value !== '.' && value !== '..' && !/[\\/\0]/.test(value),
    message: '名称不能是 . 或 ..，且不能包含 / 或 \\',
    trigger: 'blur',
  },
]
