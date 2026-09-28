export default {
  extends: ['@commitlint/config-conventional'],
  rules: {
    'type-enum': [
      2,
      'always',
      [
        'feat',
        'fix',
        'docs',
        'style',
        'refactor',
        'perf',
        'test',
        'build',
        'ci',
        'chore',
        'revert',
      ],
    ],
    'subject-case': [0],
    'subject-empty': [0],
    'type-empty': [0],
  },
  parserPreset: {
    parserOpts: {
      headerPattern: /^(?::\w+:|(?:\u00a9|\u00ae|[\u2000-\u3300]|\ud83c[\ud000-\udfff]|\ud83d[\ud000-\udfff]|\ud83e[\ud000-\udfff]))\s?(?:(\w+)(?:\(([^)]+)\))?:\s?)?(.+)$/,
      headerCorrespondence: ['type', 'scope', 'subject'],
    },
  },
};
