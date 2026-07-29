import { createPinia, setActivePinia } from 'pinia'
import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it } from 'vitest'
import TransferView from './TransferView.vue'

const FnOSIcon = defineComponent({
  props: { size: { type: Number, default: 18 } },
  template: '<svg data-icon="fnos" :width="size" />',
})

describe('Transfer View', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('uses the packaged fnOS icon in the browser device bar', () => {
    const wrapper = mount(TransferView, {
      global: {
        stubs: {
          FnOSIcon,
          FileBrowser: true,
          LocalDirectoryPicker: true,
          TSelect: true,
          TDialog: true,
          TAlert: true,
          TInput: true,
          TForm: true,
          TFormItem: true,
          TCheckbox: true,
          TRadio: true,
          TRadioGroup: true,
        },
      },
    })

    expect(wrapper.get('.browser-devicebar-icon [data-icon="fnos"]').attributes('width')).toBe('18')
  })
})
