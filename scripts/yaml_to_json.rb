require 'json'
require 'psych'

YAML_STRING_TAG = 'tag:yaml.org,2002:str'
YAML_SCANNER = Psych::ScalarScanner.new(Psych::ClassLoader::Restricted.new([], []))

def validate_node(node)
  if node.respond_to?(:tag) && node.tag && node.tag != YAML_STRING_TAG
    raise "unsupported YAML tag: #{node.tag}"
  end
  if node.is_a?(Psych::Nodes::Mapping)
    keys = {}
    node.children.each_slice(2) do |key, value|
      raise 'YAML mapping keys must be scalars' unless key.is_a?(Psych::Nodes::Scalar)
      validate_node(key)
      decoded = key.plain && key.tag.nil? ? YAML_SCANNER.tokenize(key.value) : key.value
      raise 'YAML mapping keys must be strings' unless decoded.is_a?(String)
      raise "duplicate YAML key: #{decoded}" if keys.key?(decoded)
      raise 'YAML merge keys are unsupported' if decoded == '<<'
      keys[decoded] = true
      validate_node(value)
    end
  elsif node.respond_to?(:children) && node.children
    node.children.each { |child| validate_node(child) }
  end
end

begin
  source = STDIN.read
  stream = Psych.parse_stream(source)
  raise 'expected exactly one YAML document' unless stream.children.length == 1
  validate_node(stream)
  value = Psych.safe_load(source, permitted_classes: [], permitted_symbols: [], aliases: false)
  raise 'YAML document must be a mapping' unless value.is_a?(Hash)
  puts JSON.generate(value)
rescue StandardError => error
  warn error.message
  exit 1
end